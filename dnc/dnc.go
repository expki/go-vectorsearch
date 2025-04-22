package dnc

import (
	"context"
	"errors"
	"fmt"
	"os"
	"runtime"
	"sync/atomic"
	"time"

	"github.com/expki/go-vectorsearch/compute"
	"github.com/expki/go-vectorsearch/config"
	"github.com/expki/go-vectorsearch/database"
	"github.com/expki/go-vectorsearch/logger"
	"github.com/vbauerster/mpb/v8"
	"github.com/vbauerster/mpb/v8/decor"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/plugin/dbresolver"
)

// TODO: limit memory usage
var queue = make(chan struct{}, max(1, runtime.NumCPU()))

func KMeansDivideAndConquer(ctx context.Context, db *database.Database, categoryID uint64, folderPath string) (err error) {
	// get embedding count
	var total int64
	err = db.WithContext(ctx).Clauses(dbresolver.Read).
		Model(&database.Embedding{}).
		Joins("INNER JOIN documents ON documents.id = embeddings.document_id").
		Where("documents.category_id = ?", categoryID).
		Count(&total).
		Error
	if err == nil {
	} else if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, os.ErrDeadlineExceeded) {
		return err
	} else {
		return errors.Join(errors.New("failed to count documents"), err)
	}

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
	dataWriter, err := newDataset(&atomic.Int64{}, len(embedding.Vector), folderPath)
	if err != nil {
		return errors.Join(errors.New("failed to create file writer"), err)
	}

	multibar := mpb.NewWithContext(ctx)

	// read all data
	type result struct {
		ID     uint64
		Vector database.VectorField
	}
	bar := multibar.AddBar(
		total,
		mpb.PrependDecorators(
			decor.Name("Read embeddings: "),
			decor.CountersNoUnit("%d / %d"),
		),
		mpb.AppendDecorators(
			decor.EwmaETA(decor.ET_STYLE_HHMMSS, 300),
		),
	)
	start := time.Now()
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
			now := time.Now()
			bar.EwmaIncrBy(len(results), now.Sub(start))
			start = now
			return nil
		}).
		Error
	bar.EnableTriggerComplete()
	if err == nil {
	} else if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, os.ErrDeadlineExceeded) {
		return err
	} else {
		return errors.Join(errors.New("failed to read database embeddings"), err)
	}

	// divide and conquer
	X := dataWriter.Finalize(multibar, 0)
	Y := make(chan []uint8)
	concurrent := &atomic.Int64{}
	concurrent.Add(1)
	go divideNconquer(ctx, multibar, concurrent, &atomic.Uint64{}, config.CENTROID_SIZE, X, Y)

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
		if len(dbCentroids) < idx+1 {
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

	// compute
	calculate, done := compute.MatrixCosineSimilarity()
	defer done()
	centroidMatrix := compute.NewMatrix(centroids)

	// re-assing to new centroids
	type update struct {
		ID         uint64
		CentroidID uint64
		Vector     database.VectorField
	}
	bar = multibar.AddBar(
		total,
		mpb.PrependDecorators(
			decor.Name("Update embeddings: "),
			decor.CountersNoUnit("%d / %d"),
		),
		mpb.AppendDecorators(
			decor.EwmaETA(decor.ET_STYLE_HHMMSS, 300),
		),
	)
	start = time.Now()
	var updates []update
	err = db.WithContext(ctx).Clauses(dbresolver.Read).
		Model(&database.Embedding{}).
		Joins("INNER JOIN documents ON documents.id = embeddings.document_id").
		Where("documents.category_id = ?", categoryID).
		Select("embeddings.id as id, embeddings.centroid_id as centroid_id, embeddings.vector as vector").
		FindInBatches(&updates, config.BATCH_SIZE_DATABASE, func(tx *gorm.DB, batch int) (err error) {
			// caclulate nearest centroids
			data := make([][]uint8, len(updates))
			for idx, embedding := range updates {
				data[idx] = embedding.Vector
			}
			dataMatrix := compute.NewMatrix(data)
			_, centroidIndexes := calculate(centroidMatrix.Clone(), dataMatrix)

			// group embeddings by nearest centroids
			updateMap := make(map[uint64][]uint64, len(centroids))
			for dataIdx, centroidIdx := range centroidIndexes {
				centroidID := dbCentroids[centroidIdx].ID
				update := updates[dataIdx]
				if update.CentroidID == centroidID {
					continue
				}
				current, ok := updateMap[centroidID]
				if !ok {
					current = make([]uint64, 0)
				}
				updateMap[centroidID] = append(current, update.ID)
			}

			// update embeddings in database
			for centroidID, embeddingIDs := range updateMap {
				if len(embeddingIDs) == 0 {
					continue
				}
				err = db.WithContext(ctx).Clauses(dbresolver.Write).
					Model(&database.Embedding{}).
					Where("id IN ?", embeddingIDs).
					Update("centroid_id", centroidID).
					Error
				if err == nil {
				} else if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, os.ErrDeadlineExceeded) {
					return err
				} else {
					return errors.Join(errors.New("failed to update database embeddings"), err)
				}
			}

			// increment progress bar
			now := time.Now()
			bar.EwmaIncrBy(len(updates), now.Sub(start))
			start = now
			return nil
		}).
		Error
	bar.EnableTriggerComplete()
	if err == nil {
	} else if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, os.ErrDeadlineExceeded) {
		return err
	} else {
		return errors.Join(errors.New("failed to read database embeddings"), err)
	}

	multibar.Wait()
	logger.Sugar().Info("Refresh centroids completed")
	return nil
}

// divide X into k subsets until target is achived
func divideNconquer(ctx context.Context, multibar *mpb.Progress, concurrent *atomic.Int64, instance *atomic.Uint64, targetSize uint64, X *dataset, Y chan<- []uint8) {
	queue <- struct{}{}
	defer func() {
		X.Close()
		<-queue
		if concurrent.Add(-1) <= 0 {
			close(Y)
		}
	}()

	id := instance.Add(1)

	// check if target is met or context is canceled
	if X.total <= targetSize || ctx.Err() != nil {
		Y <- X.centroid
		return
	}

	// create sample
	data := sample(multibar, id, X.ReadRow, int(X.total), config.SAMPLE_SIZE)
	X.Reset()

	// create centroids
	centroids := kMeans(
		multibar,
		id,
		data,
		min(
			config.SPLIT_SIZE,
			max(
				2,
				int(X.total/targetSize),
			),
		),
	)
	centroidsMatrix := compute.NewMatrix(centroids)

	// create dataset writers
	dataWriterList := make([]*createDataset, len(centroids))
	var err error
	for idx := range len(centroids) {
		dataWriterList[idx], err = newDataset(concurrent, X.vectorsize, X.folderpath)
		if err != nil {
			logger.Sugar().Fatalf("create data subset writer exception: %v", err)
		}
	}

	// create new cosine similarity graph
	cosineSim, closeGraph := compute.MatrixCosineSimilarity()
	defer closeGraph()

	// progress bar
	bar := multibar.AddBar(
		int64(X.total),
		mpb.PrependDecorators(
			decor.Name(fmt.Sprintf("%d split embeddings: ", id)),
			decor.CountersNoUnit("%d / %d"),
		),
		mpb.BarRemoveOnComplete(),
	)

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
		bar.IncrBy(len(idxList))
		minibatch = make([][]uint8, 0, config.BATCH_SIZE_CACHE)
	}

	if len(minibatch) > 0 {
		dataMatrix := compute.NewMatrix(minibatch)
		_, idxList := cosineSim(centroidsMatrix.Clone(), dataMatrix)
		for idx, nearestCentroidIdx := range idxList {
			dataWriterList[nearestCentroidIdx].WriteRow(minibatch[idx])
		}
		bar.IncrBy(len(idxList))
	}
	bar.EnableTriggerComplete()

	// divide and conquer
	for _, dataWriter := range dataWriterList {
		subsetX := dataWriter.Finalize(multibar, id)
		concurrent.Add(1)
		go divideNconquer(ctx, multibar, concurrent, instance, targetSize, subsetX, Y)
	}

	return
}
