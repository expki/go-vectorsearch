package database

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math"
	"math/rand/v2"
	"os"
	"sort"
	"time"

	"github.com/expki/go-vectorsearch/compute"
	"github.com/expki/go-vectorsearch/config"
	"github.com/schollz/progressbar/v3"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/plugin/dbresolver"
)

func (d *Database) KMeansCentroidAssignment(appCtx context.Context, categoryID uint64) (err error) {
	// Calculate k centroids
	var countDocuments int64
	err = d.DB.WithContext(appCtx).Clauses(dbresolver.Read).Model(&Document{}).Where("category_id = ?", categoryID).Count(&countDocuments).Error
	if err == nil {
	} else if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, os.ErrDeadlineExceeded) {
		return err
	} else {
		return errors.Join(errors.New("failed to count documents"), err)
	}
	k := int(math.Ceil(float64(countDocuments) / float64(config.CENTROID_SIZE)))

	// Get current centroids
	var centroids []Centroid
	err = d.DB.WithContext(appCtx).Clauses(dbresolver.Read).Where("category_id = ?", categoryID).Find(&centroids).Error
	if err == nil {
	} else if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, os.ErrDeadlineExceeded) {
		return err
	} else {
		return errors.Join(errors.New("failed to get current centroids"), err)
	}

	// Remove small centroids
	centroids, err = dropSmallCentroids(appCtx, d, centroids)
	if err == nil {
	} else if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, os.ErrDeadlineExceeded) {
		return err
	} else {
		return errors.Join(errors.New("failed to drop small centroids"), err)
	}

	// Check if we already have enough centroids
	if len(centroids) >= k {
		return nil
	}

	// create cache of documents
	cache, err := d.newCache(appCtx, categoryID)
	if err != nil {
		return errors.Join(errors.New("failed to create cache"), err)
	}
	defer cache.Close()

	// Add new random centroids
	randomIndexSet := make(map[int]bool, k-len(centroids))
	randomIndexList := make([]int, 0, k-len(centroids))
	for len(randomIndexList) < k-len(centroids) {
		randomIndex := rand.IntN(cache.total)
		if _, exists := randomIndexSet[randomIndex]; !exists {
			randomIndexSet[randomIndex] = true
			randomIndexList = append(randomIndexList, randomIndex)
		}
	}
	sort.Ints(randomIndexList)
	rowReader, closeReader := cache.readRows()
	var currentIndex int = 0
	for _, vectorIndex := range randomIndexList {
		for currentIndex < vectorIndex {
			rowReader()
			currentIndex++
		}
		newCentroid := Centroid{
			Vector:     rowReader(),
			CategoryID: categoryID,
		}
		currentIndex++
		centroids = append(centroids, newCentroid)
	}
	closeReader()

	// convert centroids to matrix
	matrixQuantizedCentroids := make([][]uint8, k)
	for idx, centroid := range centroids {
		matrixQuantizedCentroids[idx] = centroid.Vector
	}

	// create new cosine similarity graph
	cosineSimiarity, closeGraph := compute.MatrixCosineSimilarity()
	defer closeGraph()

	// Loop until convergence
	bar := progressbar.Default(-1, "K-Means Clustering")
	var converged bool
	for n := 0; n < 100 && !converged; n++ {
		if countDocuments > 100_000 {
			bar.Describe(fmt.Sprintf("K-Means Clustering (%d/100)", n))
		}
		matrixCentroids := compute.NewMatrix(matrixQuantizedCentroids)

		// initialize new centroids
		newCentroidsMeanVectors := make([][]float32, k)
		newCentroidsSumVectors := make([][]float32, k)
		newCentroidsDocumentCount := make([]int, k)
		for idx := range newCentroidsSumVectors {
			newCentroidsMeanVectors[idx] = make([]float32, len(matrixQuantizedCentroids[0]))
			newCentroidsSumVectors[idx] = make([]float32, len(matrixQuantizedCentroids[0]))
		}

		// read documents from cache
		rowReader, readCloser := cache.readRows()
		done := false
		for !done {
			// add documents to matrix
			documentQuantizedMatrix := make([][]uint8, config.BATCH_SIZE_CACHE)
			for idx := range config.BATCH_SIZE_CACHE {
				vector := rowReader()
				if vector == nil {
					documentQuantizedMatrix = documentQuantizedMatrix[:idx] // trim the slice to remove nil values
					done = true
					break
				}
				documentQuantizedMatrix[idx] = vector
			}
			matrixDocuments := compute.NewMatrix(documentQuantizedMatrix)

			// calculate nearest centroids for each document
			_, centroidIdList := cosineSimiarity(matrixCentroids.Clone(), matrixDocuments)

			// accumulate vectors and count documents for new centroids
			for idx, centroidIdx := range centroidIdList {
				documentVector := compute.DequantizeVector(documentQuantizedMatrix[idx], -1, 1)
				for j, val := range documentVector {
					newCentroidsSumVectors[centroidIdx][j] += val
				}
				newCentroidsDocumentCount[centroidIdx]++
			}

			// update progress bar
			bar.Add(len(documentQuantizedMatrix))
		}
		readCloser()

		// calculate mean vectors for new centroids
		for idx, count := range newCentroidsDocumentCount {
			for j, total := range newCentroidsSumVectors[idx] {
				newCentroidsMeanVectors[idx][j] = total / float32(count)
			}
		}

		// quantize new centroids
		newMatrixQuantizedCentroids := compute.QuantizeMatrix(newCentroidsMeanVectors, -1, 1)

		// check if converged
		for idx, newCentroid := range newMatrixQuantizedCentroids {
			centroid := matrixQuantizedCentroids[idx]
			converged = bytes.Equal(newCentroid, centroid)
			if !converged {
				break
			}
		}

		// set new centroids
		matrixQuantizedCentroids = newMatrixQuantizedCentroids
	}
	bar.Close()

	// Update centroid vectors
	for idx, vector := range matrixQuantizedCentroids {
		centroids[idx].Vector = vector
		centroids[idx].LastUpdated = time.Now()
	}

	// Save new centroids in database
	err = d.DB.WithContext(appCtx).Clauses(dbresolver.Write).Save(&centroids).Error
	if err == nil {
	} else if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, os.ErrDeadlineExceeded) {
		return err
	} else {
		return errors.Join(errors.New("failed to save new/updated centroids in database"), err)
	}

	// Assign documents to new centroids
	bar = progressbar.Default(int64(countDocuments), "Reassigning Documents to New Centroids")
	matrixCentroids := compute.NewMatrix(matrixQuantizedCentroids)
	var documents []Document
	err = d.DB.WithContext(appCtx).Clauses(dbresolver.Read).Where("category_id = ?", categoryID).Select("id", "vector", "centroid_id").FindInBatches(&documents, config.BATCH_SIZE_DATABASE, func(tx *gorm.DB, batch int) (err error) {
		// convert documents to matrix
		documentQuantizedMatrix := make([][]uint8, len(documents))
		for idx, document := range documents {
			documentQuantizedMatrix[idx] = document.Vector
		}
		matrixDocuments := compute.NewMatrix(documentQuantizedMatrix)

		// calculate nearest centroids for each document
		_, centroidIdList := cosineSimiarity(matrixCentroids.Clone(), matrixDocuments)

		// assign documents to new centroids
		centroidDocuments := make([][]uint64, k)
		for documentIdx, centroidIdx := range centroidIdList {
			centroid := centroids[centroidIdx]
			document := documents[documentIdx]
			if document.CentroidID == centroid.ID {
				bar.Add(1)
				continue
			}
			centroidDocuments[centroidIdx] = append(centroidDocuments[centroidIdx], document.ID)
		}

		// update document centroids in database
		for centroidIdx, documentIds := range centroidDocuments {
			if len(documentIds) == 0 {
				continue
			}
			centroid := centroids[centroidIdx]
			err = d.DB.WithContext(appCtx).Clauses(dbresolver.Write).Model(&Document{}).Where("id IN ?", documentIds).Update("centroid_id", centroid.ID).Error
			if err == nil {
			} else if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, os.ErrDeadlineExceeded) {
				return err
			} else {
				return errors.Join(errors.New("failed to update document centroids in database"), err)
			}

			bar.Add(len(centroidDocuments))
		}
		return nil
	}).Error
	if err == nil {
	} else if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, os.ErrDeadlineExceeded) {
		return err
	} else {
		return errors.Join(errors.New("failed to calculate nearest centroids for documents"), err)
	}

	return nil
}

func dropSmallCentroids(ctx context.Context, d *Database, centroids []Centroid) (keepCentroids []Centroid, err error) {
	if len(centroids) == 0 {
		return centroids, nil
	}
	// seperate centroids by keep and disgard
	keepCentroids = make([]Centroid, 0, len(centroids)-1)
	dropCentroids := make([]Centroid, 0, len(centroids)-1)
	for _, centroid := range centroids {
		// Get number of document in centroid
		var centroidDocuments int64
		err = d.DB.WithContext(ctx).Clauses(dbresolver.Read).Model(&Document{}).Where("centroid_id = ?", centroid.ID).Count(&centroidDocuments).Error
		if err == nil {
		} else if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, os.ErrDeadlineExceeded) {
			return nil, err
		} else {
			return nil, errors.Join(errors.New("failed to count document for centroid"), err)
		}
		// Check size is suffiecient
		if centroidDocuments >= (config.CENTROID_SIZE / 10) {
			keepCentroids = append(keepCentroids, centroid)
		} else {
			dropCentroids = append(dropCentroids, centroid)
		}
	}
	if len(dropCentroids) == 0 || len(keepCentroids) == 0 {
		return centroids, nil
	}

	// convert centroids to matrix
	matrixQuantizedKeepCentroids := make([][]uint8, len(keepCentroids))
	for idx, centroid := range keepCentroids {
		matrixQuantizedKeepCentroids[idx] = centroid.Vector
	}
	matrixKeepCentroids := compute.NewMatrix(matrixQuantizedKeepCentroids)

	// create new cosine similarity graph
	cosineSimiarity, closeGraph := compute.MatrixCosineSimilarity()
	defer closeGraph()

	// drop centroids
	bar := progressbar.Default(int64(len(dropCentroids)), "Dropping small centroids")
	defer bar.Close()
	for _, centroid := range dropCentroids {
		err = func(centroid Centroid) error {
			// lock dropping centroids to prevent adding documents to centroid
			tx := d.WithContext(ctx).Clauses(dbresolver.Write)
			if d.provider == config.DatabaseProvider_PostgreSQL {
				tx = tx.Clauses(clause.Locking{
					Strength: "UPDATE",
					Table:    clause.Table{Name: clause.CurrentTable},
				}).Begin()
			}
			err = tx.First(&centroid, "id = ?", centroid.ID).Error
			if err == nil {
			} else if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, os.ErrDeadlineExceeded) {
				if d.provider == config.DatabaseProvider_PostgreSQL {
					tx.Rollback()
				}
				return err
			} else {
				if d.provider == config.DatabaseProvider_PostgreSQL {
					tx.Rollback()
				}
				return errors.Join(errors.New("failed to retrieve the centroid to drop"), err)
			}

			// reassign centroid documents
			var documents []Document
			err = d.DB.WithContext(ctx).Clauses(dbresolver.Read).Where("centroid_id = ?", centroid.ID).Select("id", "vector", "centroid_id").FindInBatches(&documents, config.BATCH_SIZE_DATABASE, func(tx *gorm.DB, batch int) (err error) {
				// convert documents to matrix
				documentQuantizedMatrix := make([][]uint8, len(documents))
				for idx, document := range documents {
					documentQuantizedMatrix[idx] = document.Vector
				}
				matrixDocuments := compute.NewMatrix(documentQuantizedMatrix)

				// calculate nearest centroids for each document
				_, centroidIdList := cosineSimiarity(matrixKeepCentroids.Clone(), matrixDocuments)

				// assign documents to new centroids
				centroidDocuments := make([][]uint64, len(keepCentroids))
				for documentIdx, centroidIdx := range centroidIdList {
					centroid := centroids[centroidIdx]
					document := documents[documentIdx]
					if document.CentroidID == centroid.ID {
						continue
					}
					centroidDocuments[centroidIdx] = append(centroidDocuments[centroidIdx], document.ID)
				}

				// update document centroids in database
				for centroidIdx, documentIds := range centroidDocuments {
					if len(documentIds) == 0 {
						continue
					}
					centroid := keepCentroids[centroidIdx]
					err = d.DB.WithContext(ctx).Clauses(dbresolver.Write).Model(&Document{}).Where("id IN ?", documentIds).Update("centroid_id", centroid.ID).Error
					if err == nil {
					} else if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, os.ErrDeadlineExceeded) {
						return err
					} else {
						return errors.Join(errors.New("failed to update document centroids in database"), err)
					}
				}
				return nil
			}).Error
			if err == nil {
			} else if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, os.ErrDeadlineExceeded) {
				if d.provider == config.DatabaseProvider_PostgreSQL {
					tx.Rollback()
				}
				return err
			} else {
				if d.provider == config.DatabaseProvider_PostgreSQL {
					tx.Rollback()
				}
				return errors.Join(errors.New("failed to calculate nearest keep centroids for documents"), err)
			}

			// Drop centroid
			err = tx.Delete(&centroid).Error
			if err == nil {
			} else if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, os.ErrDeadlineExceeded) {
				if d.provider == config.DatabaseProvider_PostgreSQL {
					tx.Rollback()
				}
				return err
			} else {
				if d.provider == config.DatabaseProvider_PostgreSQL {
					tx.Rollback()
				}
				return errors.Join(errors.New("failed to calculate nearest keep centroids for documents"), err)
			}

			// Release lock
			if d.provider == config.DatabaseProvider_PostgreSQL {
				tx.Commit()
			}
			return nil
		}(centroid)
		if err != nil {
			return nil, err
		}
		bar.Add(1)
	}

	return keepCentroids, nil
}
