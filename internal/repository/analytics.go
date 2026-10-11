package repository

import (
	"time"

	"thyris-sz/internal/database"
	"thyris-sz/internal/models"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// UpsertAnalyticsHourly adds the given counts to analytics_hourly.
//
// A row is created when none exists for (bucket_hour, kind, name). When one
// already exists, the new counts are ADDED to it, never written over it, so
// two API replicas flushing the same hour do not erase each other's counts.
//
// It is one SQL statement, so it either writes every row or none. The caller
// can therefore retry the whole batch after an error without counting
// anything twice. Every key in rows must be unique, which the in-memory
// buffer guarantees.
func UpsertAnalyticsHourly(rows []models.AnalyticsHourly) error {
	if len(rows) == 0 {
		return nil
	}

	return database.DB.Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "bucket_hour"},
			{Name: "kind"},
			{Name: "name"},
		},
		DoUpdates: clause.Assignments(map[string]interface{}{
			"total":   gorm.Expr("analytics_hourly.total + EXCLUDED.total"),
			"blocked": gorm.Expr("analytics_hourly.blocked + EXCLUDED.blocked"),
		}),
	}).Create(&rows).Error
}

// DeleteAnalyticsHourlyBefore permanently removes every counter whose hour
// started before cutoff, and returns how many rows were removed. The table
// has no soft-delete column, so the rows are really gone.
func DeleteAnalyticsHourlyBefore(cutoff time.Time) (int64, error) {
	result := database.DB.Where("bucket_hour < ?", cutoff).Delete(&models.AnalyticsHourly{})
	return result.RowsAffected, result.Error
}
