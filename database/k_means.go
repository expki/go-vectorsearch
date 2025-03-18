package database

import (
	"bytes"
	"context"
	"errors"
	"math"
	"os"

	"github.com/expki/go-vectorsearch/compute"
	"github.com/expki/go-vectorsearch/config"
	"github.com/schollz/progressbar/v3"
	"gorm.io/gorm"
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
	k := int(math.Ceil(float64(countDocuments) / float64(config.MAX_CENTROID_SIZE)))

	// Get current centroids
	var centroids []Centroid
	err = d.DB.WithContext(appCtx).Clauses(dbresolver.Read).Where("category_id = ?", categoryID).Find(&centroids).Error
	if err == nil {
	} else if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, os.ErrDeadlineExceeded) {
		return err
	} else {
		return errors.Join(errors.New("failed to get current centroids"), err)
	}
	if len(centroids) >= k { // already have enough centroids
		return nil
	}

	// Add new random centroids
	var randomDocuments []Document
	err = d.DB.WithContext(appCtx).Clauses(dbresolver.Read).Where("category_id = ?", categoryID).Select("vector").Order("RANDOM()").Limit(k - len(centroids)).Find(&randomDocuments).Error
	if err == nil {
	} else if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, os.ErrDeadlineExceeded) {
		return
	} else {
		return errors.Join(errors.New("failed to get random documents"), err)
	}
	for _, doc := range randomDocuments {
		centroids = append(centroids, Centroid{
			Vector:     doc.Vector,
			CategoryID: categoryID,
		})
	}

	// convert centroids to matrix
	matrixQuantizedCentroids := make([][]uint8, k)
	for idx, centroid := range centroids {
		matrixQuantizedCentroids[idx] = centroid.Vector
	}

	// Loop until convergence
	bar := progressbar.Default(-1, "K-Means Clustering")
	var converged bool
	for !converged {
		matrixCentroids := compute.NewMatrix(matrixQuantizedCentroids)

		// initialize new centroids
		newCentroidsMeanVectors := make([][]float32, k)
		newCentroidsSumVectors := make([][]float32, k)
		newCentroidsDocumentCount := make([]int, k)
		for idx := range newCentroidsSumVectors {
			newCentroidsMeanVectors[idx] = make([]float32, len(matrixQuantizedCentroids[0]))
			newCentroidsSumVectors[idx] = make([]float32, len(matrixQuantizedCentroids[0]))
		}

		// retrieve category documents in batches
		var documents []Document
		err = d.DB.WithContext(appCtx).Clauses(dbresolver.Read).Where("category_id = ?", categoryID).Select("id", "vector").FindInBatches(&documents, config.BATCH_SIZE_DATABASE, func(tx *gorm.DB, batch int) (err error) {
			// convert documents to matrix
			documentQuantizedMatrix := make([][]uint8, len(documents))
			for idx, document := range documents {
				documentQuantizedMatrix[idx] = document.Vector
			}
			matrixDocuments := compute.NewMatrix(documentQuantizedMatrix)

			// calculate nearest centroids for each document
			_, centroidIdList := matrixCentroids.Clone().CosineSimilarity(matrixDocuments)

			// accumulate vectors and count documents for new centroids
			for idx, centroidIdx := range centroidIdList {
				documentVector := compute.DequantizeVector(documents[idx].Vector, -1, 1)
				for j, val := range documentVector {
					newCentroidsSumVectors[centroidIdx][j] += val
				}
				newCentroidsDocumentCount[centroidIdx]++
			}

			bar.Add(len(documents))
			return nil
		}).Error
		if err == nil {
		} else if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, os.ErrDeadlineExceeded) {
			return err
		} else {
			return errors.Join(errors.New("failed to calculate nearest centroids for documents"), err)
		}

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
		_, centroidIdList := matrixCentroids.Clone().CosineSimilarity(matrixDocuments)

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
			err = tx.Model(&Document{}).Where("id IN ?", documentIds).Update("centroid_id", centroid.ID).Error
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
