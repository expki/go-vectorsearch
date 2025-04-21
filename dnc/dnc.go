package dnc

import (
	"context"
	"errors"
	"os"
	"runtime"
	"sync/atomic"

	"github.com/expki/go-vectorsearch/compute"
	"github.com/expki/go-vectorsearch/config"
	"github.com/expki/go-vectorsearch/database"
	"github.com/expki/go-vectorsearch/logger"
	"github.com/schollz/progressbar/v3"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/plugin/dbresolver"
)

var queue = make(chan struct{}, runtime.NumCPU())

func KMeansDivideAndConquer(ctx context.Context, db *database.Database, categoryID uint64, folderPath string) (err error) {
	// get vector size
	var embedding database.Embedding
	err = db.WithContext(ctx).Clauses(dbresolver.Read).
		Select("vector").
		Take(&embedding).
		Error
	if err == nil {
	} else if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, os.ErrDeadlineExceeded) {
		return err
	} else {
		return errors.Join(errors.New("failed to get embedding"), err)
	}

	// create dataset writer
	dataWriter, err := newDataset(len(embedding.Vector), folderPath)
	if err != nil {
		return errors.Join(errors.New("failed to create file writer"), err)
	}

	// read all data
	type result struct {
		ID     uint64
		Vector database.VectorField
	}
	bar := progressbar.Default(-1, "Read database embeddings")
	var results []result
	err = db.WithContext(ctx).Clauses(dbresolver.Read).
		Model(&database.Embedding{}).
		Joins("INNER JOIN documents ON documents.id = embeddings.document_id").
		Where("documents.category_id = ?", categoryID).
		Select("embeddings.id as id, embeddings.vector as vector").
		FindInBatches(&results, config.BATCH_SIZE_DATABASE, func(tx *gorm.DB, batch int) (err error) {
			for _, item := range results {
				dataWriter.WriteRow(item.Vector)
			}
			bar.Add(len(results))
			return nil
		}).
		Error
	bar.Close()
	if err == nil {
	} else if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, os.ErrDeadlineExceeded) {
		return err
	} else {
		return errors.Join(errors.New("failed to read database embeddings"), err)
	}

	// divide and conquer
	X := dataWriter.Finalize()
	Y := make(chan []uint8)
	itteration := &atomic.Int64{}
	itteration.Add(1)
	go divideNconquer(ctx, itteration, config.CENTROID_SIZE, X, Y)

	// retrieve new centroids
	centroids := make([][]uint8, 0)
	for centroid := range Y {
		centroids = append(centroids, centroid)
	}

	// retrieve current centroids
	var dbCentroids []database.Centroid
	err = db.WithContext(ctx).Clauses(dbresolver.Read).
		Take(&dbCentroids, "category_id = ?", categoryID).
		Error
	if err == nil {
	} else if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, os.ErrDeadlineExceeded) {
		return err
	} else {
		return errors.Join(errors.New("failed to read database centroids"), err)
	}

	// update database centroids
	for idx, centroid := range centroids {
		if len(dbCentroids) == idx+1 {
			dbCentroids = append(dbCentroids, database.Centroid{
				CategoryID: categoryID,
			})
		}
		dbCentroids[idx].Vector = centroid
	}
	err = db.WithContext(ctx).Clauses(dbresolver.Write).
		Omit(clause.Associations).
		Save(&dbCentroids).
		Error
	if err == nil {
	} else if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, os.ErrDeadlineExceeded) {
		return err
	} else {
		return errors.Join(errors.New("failed to update database centroids"), err)
	}

	// re-assing to new centroids
	type update struct {
		ID         uint64
		CentroidID uint64
		Vector     database.VectorField
	}
	bar = progressbar.Default(-1, "Update database embeddings")
	var updates []update
	err = db.WithContext(ctx).Clauses(dbresolver.Read).
		Model(&database.Embedding{}).
		Joins("INNER JOIN documents ON documents.id = embeddings.document_id").
		Where("documents.category_id = ?", categoryID).
		Select("embeddings.id as id, embeddings.centroid_id as centroid_id, embeddings.vector as vector").
		FindInBatches(&updates, config.BATCH_SIZE_DATABASE, func(tx *gorm.DB, batch int) (err error) {
			// todo: reassign documents
			//db.Model(&User{}).Where("id IN ?", ids).Update("email", "newemail@example.com")
			bar.Add(len(updates))
			return nil
		}).
		Error
	bar.Close()
	if err == nil {
	} else if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, os.ErrDeadlineExceeded) {
		return err
	} else {
		return errors.Join(errors.New("failed to read database embeddings"), err)
	}
	return nil
}

// divide X into k subsets until target is achived
func divideNconquer(ctx context.Context, itteration *atomic.Int64, targetSize uint64, X *dataset, Y chan<- []uint8) {
	queue <- struct{}{}
	defer func() {
		X.Close()
		<-queue
		if itteration.Add(-1) <= 0 {
			close(Y)
		}
	}()

	// check if target is met or context is canceled
	if X.total <= targetSize || ctx.Err() != nil {
		Y <- X.centroid
		return
	}

	// create sample
	data := sample(X.ReadRow, int(X.total), 50_000)
	X.Reset()

	// create centroids
	centroids := kMeans(data, 2)
	centroidsMatrix := compute.NewMatrix(centroids)

	// create dataset writers
	dataWriterList := make([]*createDataset, len(centroids))
	var err error
	for idx := range len(centroids) {
		dataWriterList[idx], err = newDataset(X.vectorsize, X.folderpath)
		if err != nil {
			logger.Sugar().Fatalf("create data subset writer exception: %v", err)
		}
	}

	// create new cosine similarity graph
	cosineSim, closeGraph := compute.MatrixCosineSimilarity()
	defer closeGraph()

	// progress bar
	bar := progressbar.Default(int64(X.total), "Dataset Centroid assignment")

	// split dataset
	minibatch := make([][]uint8, 0, config.BATCH_SIZE_CACHE)
	for {
		vector := X.ReadRow()
		if vector == nil {
			break
		}
		minibatch = append(minibatch, vector)
		if len(minibatch) < config.BATCH_SIZE_CACHE {
			continue
		}
		dataMatrix := compute.NewMatrix(minibatch)
		_, idxList := cosineSim(centroidsMatrix.Clone(), dataMatrix)
		for idx, nearestCentroidIdx := range idxList {
			dataWriterList[nearestCentroidIdx].WriteRow(minibatch[idx])
		}
		bar.Add(len(idxList))
		minibatch = make([][]uint8, 0, config.BATCH_SIZE_CACHE)
	}
	if len(minibatch) > 0 {
		dataMatrix := compute.NewMatrix(minibatch)
		_, idxList := cosineSim(centroidsMatrix.Clone(), dataMatrix)
		for idx, nearestCentroidIdx := range idxList {
			dataWriterList[nearestCentroidIdx].WriteRow(minibatch[idx])
		}
		bar.Add(len(idxList))
	}
	bar.Close()

	// divide and conquer
	for _, dataWriter := range dataWriterList {
		subsetX := dataWriter.Finalize()
		itteration.Add(1)
		go divideNconquer(ctx, itteration, targetSize, subsetX, Y)
	}

	return
}
