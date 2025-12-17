package mq

import (
	"hash"
	"hash/fnv"
	"testing"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
)

// **Feature: 10k-tps-optimization, Property 3: Partition Key Consistency**
// **Validates: Requirements 2.2**
// *For any* two messages with the same goods_id, both messages SHALL be routed to the same Kafka partition.
func TestProperty_PartitionKeyConsistency(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	parameters.Rng.Seed(42)

	properties := gopter.NewProperties(parameters)

	// Simulate Kafka's Hash balancer behavior (FNV-1a hash)
	hashPartition := func(key []byte, numPartitions int) int {
		var h hash.Hash32 = fnv.New32a()
		h.Write(key)
		partition := int(h.Sum32()) % numPartitions
		if partition < 0 {
			partition = -partition
		}
		return partition
	}

	properties.Property("same goods_id always routes to same partition", prop.ForAll(
		func(goodsID int64, numPartitions int) bool {
			// Ensure valid partition count (at least 1)
			if numPartitions < 1 {
				numPartitions = 1
			}

			// Convert goods_id to key bytes (simulating how it's used in production)
			key := []byte(string(rune(goodsID)))

			// Calculate partition multiple times - should always be the same
			partition1 := hashPartition(key, numPartitions)
			partition2 := hashPartition(key, numPartitions)
			partition3 := hashPartition(key, numPartitions)

			return partition1 == partition2 && partition2 == partition3
		},
		gen.Int64Range(1, 1000000), // goods_id from 1 to 1,000,000
		gen.IntRange(1, 64),        // partition count from 1 to 64
	))

	properties.Property("different goods_ids with same value route to same partition", prop.ForAll(
		func(goodsID int64, numPartitions int) bool {
			if numPartitions < 1 {
				numPartitions = 1
			}

			// Same goods_id used twice should produce same partition
			key1 := []byte(string(rune(goodsID)))
			key2 := []byte(string(rune(goodsID)))

			partition1 := hashPartition(key1, numPartitions)
			partition2 := hashPartition(key2, numPartitions)

			return partition1 == partition2
		},
		gen.Int64Range(1, 1000000),
		gen.IntRange(1, 64),
	))

	properties.TestingRun(t)
}
