package dnc

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/expki/go-vectorsearch/compute"
	"github.com/expki/go-vectorsearch/config"
	"github.com/expki/go-vectorsearch/database"
	"github.com/expki/go-vectorsearch/logger"
	"github.com/vbauerster/mpb/v8"
	"github.com/vbauerster/mpb/v8/decor"
	"go.uber.org/zap/zapcore"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/plugin/dbresolver"
)

// TODO: limit memory usage
// TODO: limit to 1 for SQLLite
var (
	parallel = max(1, runtime.NumCPU())
	queue    = make(chan struct{}, parallel)
)

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
	dataWriter, err := newDataset(&atomic.Int64{}, len(compute.DequantizeVectorFloat32(embedding.Vector)), folderPath)
	if err != nil {
		return errors.Join(errors.New("failed to create file writer"), err)
	}

	multibarOpts := []mpb.ContainerOption{}
	if logger.Sugar().Level() == zapcore.DebugLevel {
		multibarOpts = append(multibarOpts, mpb.WithOutput(io.Discard))
	}
	multibar := mpb.NewWithContext(ctx, multibarOpts...)

	// read all data
	type result struct {
		ID     uint64
		Vector []byte
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
	if dataWriter.total == 0 {
		logger.Sugar().Debug("no embeddings in database")
		return
	}

	// divide and conquer
	logger.Sugar().Debug("Starting Divide and Conquer")
	X := dataWriter.Finalize(multibar, 0)
	Y := make(chan []uint8)
	instance := &atomic.Uint64{}
	concurrent := &atomic.Int64{}
	concurrent.Add(1)
	go divideNconquer(ctx, multibar, concurrent, instance, config.CENTROID_SIZE, X, Y)

	// retrieve new centroids
	logger.Sugar().Debug("Waiting for results")
	centroids := make([][]uint8, 0)
	for centroid := range Y {
		centroids = append(centroids, centroid)
	}

	// retrieve current centroids
	logger.Sugar().Debug("Retrieving database centroids")
	var dbCentroids []database.Centroid
	err = db.WithContext(ctx).Clauses(dbresolver.Read).
		Find(&dbCentroids, "category_id = ?", categoryID).
		Error
	if err == nil {
	} else if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, os.ErrDeadlineExceeded) {
		return err
	} else {
		return errors.Join(errors.New("failed to read database centroids"), err)
	}

	// update database centroids
	logger.Sugar().Debug("Updating database centroid vectors")
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
	logger.Sugar().Debug("Re-assigning embeddings to updated centroids")
	type update struct {
		ID         uint64
		CentroidID uint64
		Vector     []byte
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
			var wg sync.WaitGroup
			for centroidID, embeddingIDs := range updateMap {
				if len(embeddingIDs) == 0 {
					continue
				}
				queue <- struct{}{}
				if ctx.Err() != nil {
					<-queue
					return ctx.Err()
				}
				wg.Add(1)
				go func(centroidID uint64, embeddingIDs []uint64) {
					err = db.WithContext(ctx).Clauses(dbresolver.Write).
						Model(&database.Embedding{}).
						Where("id IN ?", embeddingIDs).
						Update("centroid_id", centroidID).
						Error
					if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, os.ErrDeadlineExceeded) {
						logger.Sugar().Errorf("failed to update database embeddings (%d): %s", centroidID, err.Error())
					}
					<-queue
					wg.Done()
				}(centroidID, embeddingIDs)
			}

			// increment progress bar
			wg.Wait()
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

	// drop small
	err = dropSmallCentroids(ctx, multibar, db)
	if err == nil {
	} else if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, os.ErrDeadlineExceeded) {
		return err
	} else {
		return errors.Join(errors.New("failed to drop small centroids"), err)
	}

	// Re-center centroids
	var wg sync.WaitGroup
	for _, dbCentroid := range dbCentroids {
		queue <- struct{}{}
		if ctx.Err() != nil {
			<-queue
			break
		}
		wg.Add(1)
		go func() {
			err = recenterDbCentroid(ctx, multibar, db, dbCentroid)
			if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, os.ErrDeadlineExceeded) {
				logger.Sugar().Errorf("recenter db centroids: %s", err.Error())
			}
			<-queue
			wg.Done()
		}()
	}

	wg.Wait()
	multibar.Wait()
	logger.Sugar().Infof("Refresh centroids completed (%d)", instance.Load())
	return nil
}

// divide X into k subsets until target is achived
func divideNconquer(ctx context.Context, multibar *mpb.Progress, concurrent *atomic.Int64, instance *atomic.Uint64, targetSize uint64, initX func() *dataset, Y chan<- []uint8) {
	queue <- struct{}{}
	X := initX()
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

func recenterDbCentroid(ctx context.Context, multibar *mpb.Progress, db *database.Database, centroid database.Centroid) (err error) {
	// progress bar
	bar := multibar.AddBar(
		0,
		mpb.PrependDecorators(
			decor.Name(fmt.Sprintf("Recenter centroid %d: ", centroid.ID)),
			decor.CountersNoUnit("%d / %d"),
		),
		mpb.AppendDecorators(
			decor.EwmaETA(decor.ET_STYLE_HHMMSS, 300),
		),
	)
	defer bar.EnableTriggerComplete()

	// load centroid embeddings
	dataSum := make([]float64, len(compute.DequantizeVectorFloat64(centroid.Vector)))
	var count uint64 = 0
	var embeddings []database.Embedding
	err = db.WithContext(ctx).Clauses(dbresolver.Read).
		Where("centroid_id = ?", centroid.ID).
		Select("id", "vector").
		FindInBatches(&embeddings, config.BATCH_SIZE_DATABASE, func(tx *gorm.DB, batch int) error {
			elen := len(embeddings)
			if elen == 0 {
				return nil
			}
			for _, embedding := range embeddings {
				for idx, val := range compute.DequantizeVectorFloat64(embedding.Vector) {
					dataSum[idx] += val
				}
				count++
			}
			bar.IncrBy(elen)
			return nil
		}).
		Error
	if err == nil {
	} else if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, os.ErrDeadlineExceeded) {
		return err
	} else {
		return errors.Join(errors.New("failed to read database embeddings"), err)
	}

	// calculate mean
	for idx, val := range dataSum {
		dataSum[idx] = val / float64(count)
	}
	meanVector := compute.QuantizeVectorFloat64(dataSum)

	// update centroid vector
	centroid.Vector = meanVector
	return db.WithContext(ctx).Clauses(dbresolver.Write).
		Save(&centroid).
		Error
}

func dropSmallCentroids(ctx context.Context, multibar *mpb.Progress, db *database.Database) (err error) {
	type result struct {
		ID     uint64
		Vector []byte
		Total  int64
	}
	var results []result
	err = db.WithContext(ctx).Clauses(dbresolver.Read).
		Model(&database.Centroid{}).
		Joins("INNER JOIN embeddings ON embeddings.centroid_id = centroids.id").
		Select("centroids.id", "centroids.vector", "COUNT(*) as total").
		Group("centroids.id").Group("centroids.vector").
		Find(&results).
		Error
	if err == nil {
	} else if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, os.ErrDeadlineExceeded) {
		return err
	} else {
		return errors.Join(errors.New("failed to read centroid total embeddings"), err)
	}
	if len(results) <= 1 {
		return nil
	}
	slices.SortFunc(results, func(a, b result) int {
		return cmp.Compare(a.Total, b.Total)
	})
	var oldCentroids []result
	for idx, item := range results[:len(results)-1] {
		if item.Total < (config.CENTROID_SIZE / 10) {
			oldCentroids = results[:idx]
			results = results[idx:]
			break
		}
	}
	centroids := make([][]uint8, len(results))
	for idx, item := range results {
		centroids[idx] = item.Vector
	}
	centroidMatrix := compute.NewMatrix(centroids)

	// create new cosine similarity graph
	cosineSim, closeGraph := compute.MatrixCosineSimilarity()
	defer closeGraph()
	var wg sync.WaitGroup
	for _, oldCentroid := range oldCentroids {
		queue <- struct{}{}
		if ctx.Err() != nil {
			<-queue
			break
		}
		wg.Add(1)
		go func() {
			bar := multibar.AddBar(
				0,
				mpb.PrependDecorators(
					decor.Name(fmt.Sprintf("Dropping centroid centroid %d: ", oldCentroid.ID)),
					decor.CountersNoUnit("%d / %d"),
				),
				mpb.AppendDecorators(
					decor.EwmaETA(decor.ET_STYLE_HHMMSS, 300),
				),
			)
			defer bar.EnableTriggerComplete()
			var embeddings []database.Embedding
			err := db.WithContext(ctx).Clauses(dbresolver.Read).
				Where("centroid_id = ?", oldCentroid.ID).
				Select("id", "vector").
				FindInBatches(&embeddings, config.BATCH_SIZE_DATABASE, func(tx *gorm.DB, batch int) error {
					elen := len(embeddings)
					if elen == 0 {
						return nil
					}
					data := make([][]uint8, elen)
					for idx, embedding := range embeddings {
						data[idx] = embedding.Vector
					}
					dataMatrix := compute.NewMatrix(data)
					updates := make(map[uint64][]uint64, len(centroids))
					_, centroidIds := cosineSim(centroidMatrix.Clone(), dataMatrix)
					for embeddingIdx, centroidId := range centroidIds {
						mapId := uint64(centroidId)
						embeddingId := embeddings[embeddingIdx].ID
						list, ok := updates[mapId]
						if !ok {
							list = []uint64{embeddingId}
						} else {
							list = append(list, embeddingId)
						}
						updates[mapId] = list
					}
					for centroidId, embeddingIds := range updates {
						err = db.WithContext(ctx).Clauses(dbresolver.Write).
							Model(&database.Embedding{}).
							Where("id IN ?", embeddingIds).
							Update("centroid_id", centroidId).
							Error
						if err == nil {
						} else if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, os.ErrDeadlineExceeded) {
							return err
						} else {
							return errors.Join(errors.New("failed to update embeddings centroid"), err)
						}
					}
					bar.IncrBy(elen)
					return nil
				}).
				Error
			if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, os.ErrDeadlineExceeded) {
				logger.Sugar().Errorf("failed to update embeddings centroid: %v", err)
			}
			<-queue
			wg.Done()
		}()
	}
	wg.Wait()
	return nil
}
