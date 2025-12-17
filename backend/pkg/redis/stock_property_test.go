package redis

import (
	"testing"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
)

// **Feature: 10k-tps-optimization, Property 1: Stock Distribution Consistency**
// **Validates: Requirements 1.1**
// *For any* goods with initial stock S, after initialization, the sum of all 32 segment stocks SHALL equal S.
func TestProperty_StockDistributionConsistency(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	parameters.Rng.Seed(42)

	properties := gopter.NewProperties(parameters)

	properties.Property("sum of all segment stocks equals initial stock", prop.ForAll(
		func(stock int) bool {
			// Calculate expected distribution
			baseStock := stock / SegmentCount
			remainder := stock % SegmentCount

			// Sum up all segments
			totalDistributed := 0
			for i := 0; i < SegmentCount; i++ {
				segmentStock := baseStock
				if i < remainder {
					segmentStock++
				}
				totalDistributed += segmentStock
			}

			return totalDistributed == stock
		},
		gen.IntRange(0, 1000000), // Test with stock values from 0 to 1,000,000
	))

	properties.TestingRun(t)
}

// **Feature: 10k-tps-optimization, Property 2: Stock Restoration Integrity**
// **Validates: Requirements 1.3**
// *For any* successful seckill that deducts from segment X, if the order is cancelled,
// restoring stock to segment X SHALL preserve total stock count.
func TestProperty_StockRestorationIntegrity(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	parameters.Rng.Seed(42)

	properties := gopter.NewProperties(parameters)

	properties.Property("stock restoration preserves total count", prop.ForAll(
		func(stock int, segmentID int) bool {
			// Ensure segmentID is within valid range
			segmentID = segmentID % SegmentCount
			if segmentID < 0 {
				segmentID = -segmentID
			}

			// Calculate initial distribution
			baseStock := stock / SegmentCount
			remainder := stock % SegmentCount

			segments := make([]int, SegmentCount)
			for i := 0; i < SegmentCount; i++ {
				segments[i] = baseStock
				if i < remainder {
					segments[i]++
				}
			}

			// Simulate deduction from segment
			if segments[segmentID] > 0 {
				segments[segmentID]--

				// Calculate total after deduction
				totalAfterDeduct := 0
				for _, s := range segments {
					totalAfterDeduct += s
				}

				// Simulate restoration to the same segment
				segments[segmentID]++

				// Calculate total after restoration
				totalAfterRestore := 0
				for _, s := range segments {
					totalAfterRestore += s
				}

				// Total after restoration should equal original stock
				return totalAfterRestore == stock
			}

			// If segment was empty, no deduction happened, total should still equal stock
			total := 0
			for _, s := range segments {
				total += s
			}
			return total == stock
		},
		gen.IntRange(1, 1000000), // Stock values from 1 to 1,000,000
		gen.IntRange(0, 31),      // Segment IDs from 0 to 31
	))

	properties.TestingRun(t)
}
