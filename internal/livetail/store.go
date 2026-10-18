package livetail

import (
	"context"
	"errors"

	"github.com/redis/go-redis/v9"
)

// StreamStart is the id XREAD starts from: only entries appended after the first read.
type StreamStart string

const StreamFromNow StreamStart = "$"

// StreamEntry is one decoded stream entry.
type StreamEntry struct {
	ID   string
	Kind EntryKind
	Data []byte
}

// TailStore is the API side of the tail sink: raise the watch flag and read new entries.
type TailStore interface {
	Watch(ctx context.Context, routeID string) error
	Read(ctx context.Context, routeID, after string) ([]StreamEntry, error)
}

// RedisTailStore implements TailStore on the Redis flag key and capped stream.
type RedisTailStore struct {
	Client redis.UniversalClient
	Limits TailLimits
}

// Watch sets (or renews) the route's flag for FlagTTL.
func (s RedisTailStore) Watch(ctx context.Context, routeID string) error {
	ctx, cancel := context.WithTimeout(ctx, s.Limits.RedisTimeout)
	defer cancel()
	return s.Client.Set(ctx, TailKey{RouteID: routeID}.Flag(), string(StreamFromNow), s.Limits.FlagTTL).Err()
}

// Read blocks up to ReadBlock for entries after the given id; a timeout returns no entries.
func (s RedisTailStore) Read(ctx context.Context, routeID, after string) ([]StreamEntry, error) {
	res, err := s.Client.XRead(ctx, &redis.XReadArgs{Streams: []string{TailKey{RouteID: routeID}.Stream(), after},
		Count: s.Limits.ReadCount, Block: s.Limits.ReadBlock}).Result()
	if errors.Is(err, redis.Nil) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []StreamEntry
	for _, stream := range res {
		for _, m := range stream.Messages {
			out = append(out, EntryDecoder{}.Decode(m))
		}
	}
	return out, nil
}

// EntryDecoder turns a raw stream message into a StreamEntry.
type EntryDecoder struct{}

// Decode reads the kind and data fields (missing fields stay empty).
func (EntryDecoder) Decode(m redis.XMessage) StreamEntry {
	e := StreamEntry{ID: m.ID}
	if k, ok := m.Values[string(FieldKind)].(string); ok {
		e.Kind = EntryKind(k)
	}
	if d, ok := m.Values[string(FieldData)].(string); ok {
		e.Data = []byte(d)
	}
	return e
}
