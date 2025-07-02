package orm

import (
	"fmt"
	"sync"

	"github.com/hypernetix/hyperspot/libs/errorx"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/schema"
)

func checkPrimaryKey(model interface{}, pkField string) bool {
	// Get schema from model
	schemaType, err := schema.Parse(model, &sync.Map{}, schema.NamingStrategy{})
	if err != nil {
		return false
	}

	// Check if the model has a primary key field with the specified name
	for _, field := range schemaType.Fields {
		if field.DBName == pkField && field.PrimaryKey {
			return true
		}
	}

	// If no matching primary key field is found
	return false
}

// OrmDeleteObjs deletes records matching the filter criteria in a batched manner.
// This function addresses several GORM limitations:
// 1. GORM's default Delete() method can timeout or lock the database when deleting large sets of records
// 2. SQLite has statement limits that can cause "too many SQL variables" errors with large IN clauses
// 3. Long-running delete operations can block other database operations
// 4. DELETE with LIMIT is not a part of SQL standard, so not supported by different vendors (e.g. PostgreSQL)
//
// The implementation uses a batched approach where:
// - Records are deleted in smaller batches (controlled by batchSize) to avoid database locks
// - Each batch runs in its own transaction to minimize lock duration
// - Primary keys are fetched first, then used for targeted deletes via "IN" clauses
// - An optional limit parameter controls the maximum number of records to delete
//
// Parameters:
// - query: A prepared GORM query with any WHERE and conditions already applied
// - primaryField: The name of the primary key field
// - limit: Maximum number of records to delete (0 means use query's Limit or delete all)
// - batchSize: Maximum number of records to process in each transaction (defaults to 1000)
//
// Returns:
// - Number of records deleted
// - Any error encountered during the process
func OrmDeleteObjs(query *gorm.DB, primaryField string, limit int, batchSize int) (int64, errorx.Error) {
	// validate input
	if query == nil {
		return 0, errorx.NewErrBadRequest("invalid query")
	}
	// ensure model has specified primary key field
	stmt := query.Statement
	if stmt == nil || stmt.Model == nil {
		return 0, errorx.NewErrBadRequest("invalid query statement")
	}
	if !checkPrimaryKey(stmt.Model, primaryField) {
		return 0, errorx.NewErrBadRequest(fmt.Sprintf("model primary key `%s` doesn't exist", primaryField))
	}

	if batchSize <= 0 || batchSize > 1000 {
		batchSize = 1000 // Default to 1000 if not specified or exceeds limit
	}

	// detect LIMIT clause if no explicit limit
	if limit <= 0 {
		if stmt := query.Statement; stmt != nil {
			if c, ok := stmt.Clauses["LIMIT"]; ok {
				if lc, ok2 := c.Expression.(clause.Limit); ok2 && lc.Limit != nil && *lc.Limit > 0 {
					limit = *lc.Limit
				}
			}
		}
	}

	var totalDeleted int64
	// batch delete loop
	for {
		q := query.Session(&gorm.Session{})
		var ids []interface{}
		if err := q.Limit(batchSize).Pluck(primaryField, &ids).Error; err != nil {
			return totalDeleted, errorx.NewErrBadRequest(err.Error())
		}
		if len(ids) == 0 {
			break
		}
		if limit > 0 && totalDeleted+int64(len(ids)) > int64(limit) {
			ids = ids[:limit-int(totalDeleted)]
		}
		// Execute batch delete with retry logic
		rowsAffected, err := RetryWithResult(func() (int64, error) {
			res := query.Session(&gorm.Session{}).Where(fmt.Sprintf("%s IN ?", primaryField), ids).Delete(query.Statement.Model)
			return res.RowsAffected, res.Error
		}, DefaultRetryConfig())

		if err != nil {
			return totalDeleted, err
		}

		totalDeleted += rowsAffected
		if limit > 0 && totalDeleted >= int64(limit) {
			break
		}
	}
	return totalDeleted, nil
}

// OrmUpdateObjs updates fields to given values for all records matching the query criteria.
// This function addresses several GORM limitations:
// 1. GORM's default Updates() method doesn't handle large result sets efficiently
// 2. SQLite has statement limits that can cause "too many SQL variables" errors with large IN clauses
// 3. Long-running transactions can lead to database locks in concurrent environments
// 4. UPDATE with LIMIT is not a part of SQL standard, so not supported by different vendors (e.g. PostgreSQL)
//
// The implementation uses a batched approach where:
// - Records are processed in smaller batches (controlled by batchSize) to avoid SQLite limitations
// - Each batch runs in its own transaction to minimize lock duration
// - Primary keys are fetched first, then used for targeted updates via "IN" clauses
// - The function handles field name to column name conversion automatically
// - An optional limit parameter controls the maximum number of records to update
//
// Parameters:
// - query: A prepared GORM query with any WHERE and LIMIT conditions already applied
// - primaryField: The name of the primary key field
// - updates: Map of field names to values to update. Can contain either struct field names or DB column names
// - limit: Maximum number of records to update (0 means use query's Limit or update all)
// - batchSize: Maximum number of records to process in each transaction (defaults to 1000)
//
// Returns:
// - Number of records updated
// - Any error encountered during the process
func OrmUpdateObjs(query *gorm.DB, primaryField string, updates map[string]interface{}, limit int, batchSize int) (int64, errorx.Error) {
	// validate input
	if query == nil {
		return 0, errorx.NewErrBadRequest("invalid query")
	}
	// ensure model has specified primary key field
	stmt := query.Statement
	if stmt == nil || stmt.Model == nil {
		return 0, errorx.NewErrBadRequest("invalid query statement")
	}
	if !checkPrimaryKey(stmt.Model, primaryField) {
		return 0, errorx.NewErrBadRequest(fmt.Sprintf("model primary key `%s` doesn't exist", primaryField))
	}

	if len(updates) == 0 {
		return 0, errorx.NewErrBadRequest("no updates provided")
	}

	if batchSize <= 0 || batchSize > 1000 {
		batchSize = 1000 // Default to 1000 if not specified or exceeds limit
	}

	// detect LIMIT clause if no explicit limit
	if limit <= 0 {
		if stmt := query.Statement; stmt != nil {
			if c, ok := stmt.Clauses["LIMIT"]; ok {
				if lc, ok2 := c.Expression.(clause.Limit); ok2 && lc.Limit != nil && *lc.Limit > 0 {
					limit = *lc.Limit
				}
			}
		}
	}

	var totalUpdated int64
	var processedIds []interface{}
	var idBatches [][]interface{}
	var collectedCount int64
	// first loop: collect ID batches
	for {
		q := query.Session(&gorm.Session{})
		if len(processedIds) > 0 {
			q = q.Where(fmt.Sprintf("%s NOT IN ?", primaryField), processedIds)
		}
		var ids []interface{}
		if err := q.Limit(batchSize).Pluck(primaryField, &ids).Error; err != nil {
			return totalUpdated, errorx.NewErrBadRequest(err.Error())
		}
		if len(ids) == 0 {
			break
		}
		if limit > 0 && collectedCount+int64(len(ids)) > int64(limit) {
			ids = ids[:limit-int(collectedCount)]
		}
		idBatches = append(idBatches, ids)
		processedIds = append(processedIds, ids...)
		collectedCount += int64(len(ids))
		if limit > 0 && collectedCount >= int64(limit) {
			break
		}
	}
	// second loop: perform updates
	for _, ids := range idBatches {
		// Execute batch update with retry logic
		rowsAffected, err := RetryWithResult(func() (int64, error) {
			res := query.Session(&gorm.Session{}).Where(fmt.Sprintf("%s IN ?", primaryField), ids).Updates(updates)
			return res.RowsAffected, res.Error
		}, DefaultRetryConfig())

		if err != nil {
			return totalUpdated, err
		}

		totalUpdated += rowsAffected
	}
	return totalUpdated, nil
}
