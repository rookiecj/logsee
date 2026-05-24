package usecase

import "fmt"

const DefaultOutputLogCacheCapacity = 10000

type OutputLogCacheOptions struct {
	Capacity int
}

type OutputCacheTrimOptions struct {
	CursorOutputIndex int
	ViewportHeight    int
}

type OutputLogCache struct {
	capacity         int
	startOutputIndex int
	records          []OutputLogRecord
}

func NewOutputLogCache(options OutputLogCacheOptions) (*OutputLogCache, error) {
	capacity := options.Capacity
	if capacity == 0 {
		capacity = DefaultOutputLogCacheCapacity
	}
	if capacity < 1 {
		return nil, fmt.Errorf("output cache capacity must be 1 or greater")
	}
	return &OutputLogCache{capacity: capacity}, nil
}

func (c *OutputLogCache) Append(records []OutputLogRecord) {
	c.records = append(c.records, records...)
}

func (c *OutputLogCache) AppendAndTrim(records []OutputLogRecord, options OutputCacheTrimOptions) error {
	c.Append(records)
	return c.Trim(options)
}

func (c *OutputLogCache) Trim(options OutputCacheTrimOptions) error {
	if options.CursorOutputIndex < 0 {
		return fmt.Errorf("cursor output index must be 0 or greater")
	}
	if options.ViewportHeight < 0 {
		return fmt.Errorf("viewport height must be 0 or greater")
	}
	if len(c.records) <= c.capacity {
		return nil
	}

	protectedStart := options.CursorOutputIndex - options.ViewportHeight
	if protectedStart < 0 {
		protectedStart = 0
	}

	dropNeeded := len(c.records) - c.capacity
	maxDroppable := protectedStart - c.startOutputIndex
	if maxDroppable < 0 {
		maxDroppable = 0
	}
	if maxDroppable > len(c.records) {
		maxDroppable = len(c.records)
	}
	if dropNeeded > maxDroppable {
		return fmt.Errorf(
			"output cache cannot trim %d records without dropping protected range starting at output index %d",
			dropNeeded,
			protectedStart,
		)
	}

	c.records = append([]OutputLogRecord(nil), c.records[dropNeeded:]...)
	c.startOutputIndex += dropNeeded
	return nil
}

func (c *OutputLogCache) Len() int {
	return len(c.records)
}

func (c *OutputLogCache) StartOutputIndex() int {
	return c.startOutputIndex
}

func (c *OutputLogCache) Records() []OutputLogRecord {
	return append([]OutputLogRecord(nil), c.records...)
}
