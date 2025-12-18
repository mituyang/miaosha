package util

import (
	"sync"
	"time"
)

const (
	defaultEpoch   = int64(1704067200000) // 2024-01-01 00:00:00 UTC
	workerIDBits   = 10
	sequenceBits   = 12
	maxWorkerID    = -1 ^ (-1 << workerIDBits)
	maxSequence    = -1 ^ (-1 << sequenceBits)
	workerIDShift  = sequenceBits
	timestampShift = sequenceBits + workerIDBits
)

// 可配置的 epoch
var snowflakeEpoch = defaultEpoch

type Snowflake struct {
	mu        sync.Mutex
	timestamp int64
	workerID  int64
	sequence  int64
}

var defaultSnowflake *Snowflake

// InitSnowflake 初始化雪花算法
func InitSnowflake(workerID int64) error {
	if workerID < 0 || workerID > maxWorkerID {
		workerID = 1
	}
	defaultSnowflake = &Snowflake{workerID: workerID}
	return nil
}

// InitSnowflakeWithEpoch 初始化雪花算法（带自定义 epoch）
func InitSnowflakeWithEpoch(workerID, epoch int64) error {
	if workerID < 0 || workerID > maxWorkerID {
		workerID = 1
	}
	if epoch > 0 {
		snowflakeEpoch = epoch
	}
	defaultSnowflake = &Snowflake{workerID: workerID}
	return nil
}

// NextID 生成下一个ID
func NextID() uint64 {
	return defaultSnowflake.NextID()
}

func (s *Snowflake) NextID() uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UnixMilli()

	if now == s.timestamp {
		s.sequence = (s.sequence + 1) & maxSequence
		if s.sequence == 0 {
			for now <= s.timestamp {
				now = time.Now().UnixMilli()
			}
		}
	} else {
		s.sequence = 0
	}

	s.timestamp = now

	id := ((now - snowflakeEpoch) << timestampShift) |
		(s.workerID << workerIDShift) |
		s.sequence

	return uint64(id)
}
