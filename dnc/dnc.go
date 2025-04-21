package dnc

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/expki/go-vectorsearch/compute"
	"github.com/expki/go-vectorsearch/config"
	"github.com/expki/go-vectorsearch/database"
	"github.com/expki/go-vectorsearch/logger"
	"github.com/schollz/progressbar/v3"
	"gorm.io/gorm"
	"gorm.io/plugin/dbresolver"
)

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
	Y := divideNconquer(ctx, X, config.CENTROID_SIZE)
	X.Close()

	// todo: complete updating new centroids in database and re-assign all embeddings
	for idx, y := range Y {
		fmt.Println(idx, y.total)
		y.Close()
	}

	return nil
}

// divide X into k subsets until target is achived
func divideNconquer(ctx context.Context, X *dataset, target uint64) (Y []*dataset) {
	// check if target is met or context is canceled
	if X.total <= target || ctx.Err() != nil {
		return []*dataset{X}
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
	Y = make([]*dataset, 0, len(dataWriterList)*2)
	for _, dataWriter := range dataWriterList {
		X := dataWriter.Finalize()
		Y = append(Y, divideNconquer(ctx, X, target)...)
		X.Close()
	}

	return Y
}
