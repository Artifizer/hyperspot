package chat

import (
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/hypernetix/hyperspot/libs/api"
	"github.com/hypernetix/hyperspot/libs/orm"
)

func messagesHardDelete(days int) {
	query, errx := orm.GetBaseQuery(&ChatMessage{}, uuid.Nil, uuid.Nil, nil)
	if errx != nil {
		return
	}

	query = query.Where("created_at_ms < ? AND is_deleted = ?", time.Now().AddDate(0, 0, -days).UnixMilli(), true)
	count, errx := orm.OrmDeleteObjs(query, "sequence_id", 0, 1000)
	if errx != nil {
		logger.Error("Error hard deleting old messages: %s", errx.Error())
		return
	}

	logger.Trace("Hard deleted %d old messages", count)
}

func threadsHardDelete(days int) {
	query, errx := orm.GetBaseQuery(&ChatThread{}, uuid.Nil, uuid.Nil, nil)
	if errx != nil {
		return
	}

	query = query.Where("last_msg_at_ms < ? AND is_deleted = ?", time.Now().AddDate(0, 0, -days).UnixMilli(), true)
	count, errx := orm.OrmDeleteObjs(query, "sequence_id", 0, 1000)
	if errx != nil {
		logger.Error("Error hard deleting old threads: %s", errx.Error())
		return
	}

	logger.Trace("Hard deleted %d old threads", count)
}

func tempThreadsAndGroupsHardDelete(hours int) {
	// Hard delete all temporary threads and groups and their messages
	// Calculate cutoff time
	cutoffTime := time.Now().Add(time.Duration(-hours) * time.Hour).UnixMilli()

	// Process temporary threads
	{
		tempThreads := []ChatThread{}
		query, err := orm.GetBaseQuery(&ChatThread{}, uuid.Nil, uuid.Nil, &api.PageAPIRequest{
			PageNumber: 1,
			PageSize:   1000,
		})
		if err != nil {
			logger.Error("Error getting query for temporary threads: %v", err)
			return
		}

		// Use table name to avoid ambiguous column error
		query = query.Where("chat_threads.is_temporary = ? AND chat_threads.last_msg_at_ms < ?", true, cutoffTime)
		result := query.Find(&tempThreads)
		if result.Error != nil {
			logger.Error("Error finding temporary threads for hard delete: %v", result.Error)
			return
		}

		logger.Trace("Found %d temporary threads for hard delete", len(tempThreads))

		// Delete messages in these threads
		for _, thread := range tempThreads {
			msgQuery, err := orm.GetBaseQuery(&ChatMessage{}, uuid.Nil, uuid.Nil, nil)
			if err != nil {
				logger.Error("Error getting query for messages in thread %s: %v", thread.ID, err)
				continue
			}

			msgQuery = msgQuery.Where("thread_id = ?", thread.ID)
			count, errx := orm.OrmDeleteObjs(msgQuery, "sequence_id", 0, 1000)
			if errx != nil {
				logger.Error("Error hard deleting messages in temporary thread %s: %s", thread.ID, errx.Error())
				continue
			}

			logger.Trace("Hard deleted %d messages in temporary thread %s", count, thread.ID)
		}

		// Create a new query for the delete to avoid issues with the previous one
		threadDeleteQuery, errx := orm.GetBaseQuery(&ChatThread{}, uuid.Nil, uuid.Nil, nil)
		if errx != nil {
			logger.Error("Error getting delete query for temporary threads: %v", err)
			return
		}

		threadDeleteQuery = threadDeleteQuery.Where("is_temporary = ? AND last_msg_at_ms < ?", true, cutoffTime)
		count, errx := orm.OrmDeleteObjs(threadDeleteQuery, "sequence_id", 0, 1000)
		if errx != nil {
			logger.Error("Error hard deleting temporary threads: %s", errx.Error())
			return
		}

		logger.Trace("Hard deleted %d temporary threads", count)
	}

	// Process temporary groups
	{
		tempGroups := []ChatThreadGroup{}
		groupQuery, err := orm.GetBaseQuery(&ChatThreadGroup{}, uuid.Nil, uuid.Nil, &api.PageAPIRequest{
			PageNumber: 1,
			PageSize:   1000,
		})
		if err != nil {
			logger.Error("Error getting query for temporary groups: %v", err)
			return
		}

		// Use table name to avoid ambiguous column error
		groupQuery = groupQuery.Where("chat_thread_groups.is_temporary = ? AND chat_thread_groups.last_msg_at_ms < ?", true, cutoffTime)
		groupResult := groupQuery.Find(&tempGroups)
		if groupResult.Error != nil {
			logger.Error("Error finding temporary groups for hard delete: %v", groupResult.Error)
			return
		}

		logger.Trace("Found %d temporary groups for hard delete", len(tempGroups))

		// Create a new query for the delete to avoid issues with the previous one
		groupDeleteQuery, errx := orm.GetBaseQuery(&ChatThreadGroup{}, uuid.Nil, uuid.Nil, nil)
		if errx != nil {
			logger.Error("Error getting delete query for temporary groups: %s", errx.Error())
			return
		}

		groupDeleteQuery = groupDeleteQuery.Where("is_temporary = ? AND last_msg_at_ms < ?", true, cutoffTime)
		count, errx := orm.OrmDeleteObjs(groupDeleteQuery, "sequence_id", 0, 1000)
		if errx != nil {
			logger.Error("Error hard deleting temporary groups: %s", errx.Error())
			return
		}

		logger.Trace("Hard deleted %d temporary groups", count)
	}
}

func groupsHardDelete(days int) {
	query, errx := orm.GetBaseQuery(&ChatThreadGroup{}, uuid.Nil, uuid.Nil, nil)
	if errx != nil {
		return
	}

	query = query.Where("last_msg_at_ms < ? AND is_deleted = ?", time.Now().AddDate(0, 0, -days).UnixMilli(), true)
	count, errx := orm.OrmDeleteObjs(query, "sequence_id", 0, 1000)
	if errx != nil {
		logger.Error("Error hard deleting old thread groups: %s", errx.Error())
		return
	}

	logger.Trace("Hard deleted %d old thread groups", count)
}

func hardDeleteRetentionJob(pause time.Duration, days int, tempHours int) {
	for {
		time.Sleep(pause)

		messagesHardDelete(days)
		threadsHardDelete(days)
		groupsHardDelete(days)
		tempThreadsAndGroupsHardDelete(tempHours)
	}
}

func messagesSoftDelete(days int) {
	query, errx := orm.GetBaseQuery(&ChatMessage{}, uuid.Nil, uuid.Nil, nil)
	if errx != nil {
		return
	}

	query = query.Where("created_at_ms < ? AND is_deleted = ?", time.Now().AddDate(0, 0, -days).UnixMilli(), false)
	count, errx := orm.OrmUpdateObjs(query, "sequence_id", map[string]interface{}{"is_deleted": true}, 0, 1000)

	if errx != nil {
		logger.Error("Error soft deleting old messages: %s", errx.Error())
		return
	}

	logger.Trace("Soft deleted %d old messages", count)
}

func threadsSoftDelete(days int) {
	query, errx := orm.GetBaseQuery(&ChatThread{}, uuid.Nil, uuid.Nil, nil)
	if errx != nil {
		return
	}

	query = query.Where("last_msg_at_ms < ? AND is_deleted = ?", time.Now().AddDate(0, 0, -days).UnixMilli(), false)
	count, errx := orm.OrmUpdateObjs(query, "sequence_id", map[string]interface{}{"is_deleted": true}, 0, 1000)

	if errx != nil {
		logger.Error("Error soft deleting old threads: %s", errx.Error())
		return
	}

	logger.Trace("Soft deleted %d old threads", count)
}

func groupsSoftDelete(days int) {
	query, errx := orm.GetBaseQuery(&ChatThreadGroup{}, uuid.Nil, uuid.Nil, nil)
	if errx != nil {
		return
	}

	query = query.Where("last_msg_at_ms < ? AND is_deleted = ?", time.Now().AddDate(0, 0, -days).UnixMilli(), false)
	count, errx := orm.OrmUpdateObjs(query, "sequence_id", map[string]interface{}{"is_deleted": true}, 0, 1000)

	if errx != nil {
		logger.Error("Error soft deleting old thread groups: %s", errx.Error())
		return
	}

	logger.Trace("Soft deleted %d old thread groups", count)
}

func tempThreadsAndGroupsSoftDelete(hours int) {
	// Soft delete all temporary threads and groups and their messages
	// First, find all temporary threads that have been inactive for the specified hours
	cutoffTime := time.Now().Add(time.Duration(-hours) * time.Hour).UnixMilli()

	// Process temporary threads
	{
		tempThreads := []ChatThread{}
		query, err := orm.GetBaseQuery(&ChatThread{}, uuid.Nil, uuid.Nil, &api.PageAPIRequest{
			PageNumber: 1,
			PageSize:   1000,
		})
		if err != nil {
			logger.Error("Error getting query for temporary threads: %v", err)
			return
		}

		// Use table name to avoid ambiguous column error
		query = query.Where("chat_threads.is_temporary = ? AND chat_threads.last_msg_at_ms < ? AND chat_threads.is_deleted = ?", true, cutoffTime, false)
		result := query.Find(&tempThreads)
		if result.Error != nil {
			logger.Error("Error finding temporary threads for soft delete: %v", result.Error)
			return
		}

		logger.Trace("Found %d temporary threads for soft delete", len(tempThreads))

		// Soft delete messages in these threads
		for _, thread := range tempThreads {
			msgQuery, errx := orm.GetBaseQuery(&ChatMessage{}, uuid.Nil, uuid.Nil, nil)
			if errx != nil {
				logger.Error("Error getting query for messages in thread %s: %v", thread.ID, err)
				continue
			}

			msgQuery = msgQuery.Where("thread_id = ? AND is_deleted = ?", thread.ID, false)
			count, errx := orm.OrmUpdateObjs(msgQuery, "sequence_id", map[string]interface{}{"is_deleted": true}, 0, 1000)
			if errx != nil {
				logger.Error("Error soft deleting messages in temporary thread %s: %s", thread.ID, errx.Error())
				continue
			}

			logger.Trace("Soft deleted %d messages in temporary thread %s", count, thread.ID)
		}

		// Now soft delete the temporary threads
		// Create a new query to avoid issues with the previous one
		threadUpdateQuery, errx := orm.GetBaseQuery(&ChatThread{}, uuid.Nil, uuid.Nil, nil)
		if errx != nil {
			logger.Error("Error getting update query for temporary threads: %s", errx.Error())
			return
		}

		threadUpdateQuery = threadUpdateQuery.Where("is_temporary = ? AND last_msg_at_ms < ? AND is_deleted = ?", true, cutoffTime, false)
		count, errx := orm.OrmUpdateObjs(threadUpdateQuery, "sequence_id", map[string]interface{}{"is_deleted": true}, 0, 1000)
		if errx != nil {
			logger.Error("Error soft deleting temporary threads: %s", errx.Error())
			return
		}

		logger.Trace("Soft deleted %d temporary threads", count)
	}

	// Process temporary groups
	{
		tempGroups := []ChatThreadGroup{}
		groupQuery, err := orm.GetBaseQuery(&ChatThreadGroup{}, uuid.Nil, uuid.Nil, &api.PageAPIRequest{
			PageNumber: 1,
			PageSize:   1000,
		})
		if err != nil {
			logger.Error("Error getting query for temporary groups: %v", err)
			return
		}

		// Use table name to avoid ambiguous column error
		groupQuery = groupQuery.Where("chat_thread_groups.is_temporary = ? AND chat_thread_groups.last_msg_at_ms < ? AND chat_thread_groups.is_deleted = ?", true, cutoffTime, false)
		groupResult := groupQuery.Find(&tempGroups)
		if groupResult.Error != nil {
			logger.Error("Error finding temporary groups for soft delete: %v", groupResult.Error)
			return
		}

		logger.Trace("Found %d temporary groups for soft delete", len(tempGroups))

		// Create a new query for the update to avoid issues with the previous one
		groupUpdateQuery, errx := orm.GetBaseQuery(&ChatThreadGroup{}, uuid.Nil, uuid.Nil, nil)
		if errx != nil {
			logger.Error("Error getting update query for temporary groups: %s", errx.Error())
			return
		}

		groupUpdateQuery = groupUpdateQuery.Where("is_temporary = ? AND last_msg_at_ms < ? AND is_deleted = ?", true, cutoffTime, false)
		count, errx := orm.OrmUpdateObjs(groupUpdateQuery, "sequence_id", map[string]interface{}{"is_deleted": true}, 0, 1000)
		if errx != nil {
			logger.Error("Error soft deleting temporary groups: %s", errx.Error())
			return
		}

		logger.Trace("Soft deleted %d temporary groups", count)
	}
}

func softDeleteRetentionJob(pause time.Duration, days int, tempHours int) {
	for {
		time.Sleep(pause)

		messagesSoftDelete(days)
		threadsSoftDelete(days)
		groupsSoftDelete(days)
		tempThreadsAndGroupsSoftDelete(tempHours)
	}
}

func retentionStart(pause time.Duration, softDays int, hardDays int, tempSoftHours int, tempHardHours int) error {
	logger.Debug("Starting retention job with pause %s, softDays %d, hardDays %d, tempSoftHours %d, tempHardHours %d", pause, softDays, hardDays, tempSoftHours, tempHardHours)
	if hardDays <= softDays {
		msg := fmt.Sprintf("hardDays (%d) must be greater than softDays (%d)", hardDays, softDays)
		logger.Error(msg)
		return fmt.Errorf(msg)
	}
	if softDays > 0 {
		go softDeleteRetentionJob(pause, softDays, tempSoftHours)
	}
	if hardDays > 0 {
		go hardDeleteRetentionJob(pause, hardDays, tempHardHours)
	}
	return nil
}
