package storage

import (
	"sort"
	"sync"

	"github.com/hypasis/sync-protocol/internal/types"
)

// GapTracker tracks available and missing block ranges
type GapTracker struct {
	// Sorted list of available ranges
	availableRanges []types.BlockRange
	mu              sync.RWMutex

	// Total range we're tracking
	minBlock uint64
	maxBlock uint64
}

// NewGapTracker creates a new gap tracker
func NewGapTracker() *GapTracker {
	return &GapTracker{
		availableRanges: []types.BlockRange{},
		minBlock:        ^uint64(0), // Max uint64
		maxBlock:        0,
	}
}

// MarkAvailable marks a range of blocks as available
func (g *GapTracker) MarkAvailable(start, end uint64) {
	g.mu.Lock()
	defer g.mu.Unlock()

	if start >= end {
		return
	}

	// Update min/max
	if start < g.minBlock {
		g.minBlock = start
	}
	if end > g.maxBlock {
		g.maxBlock = end
	}

	// Add new range
	newRange := types.BlockRange{Start: start, End: end}
	g.availableRanges = append(g.availableRanges, newRange)

	// Merge overlapping ranges
	g.mergeRanges()
}

// HasBlock checks if a block is available
func (g *GapTracker) HasBlock(blockNum uint64) bool {
	g.mu.RLock()
	defer g.mu.RUnlock()

	for _, r := range g.availableRanges {
		if blockNum >= r.Start && blockNum < r.End {
			return true
		}
	}

	return false
}

// GetGaps returns all missing ranges
func (g *GapTracker) GetGaps() []types.BlockRange {
	g.mu.RLock()
	defer g.mu.RUnlock()

	if len(g.availableRanges) == 0 {
		if g.maxBlock > 0 {
			return []types.BlockRange{{Start: 0, End: g.maxBlock}}
		}
		return []types.BlockRange{}
	}

	gaps := []types.BlockRange{}

	// Gap before first available range
	if g.availableRanges[0].Start > 0 {
		gaps = append(gaps, types.BlockRange{
			Start: 0,
			End:   g.availableRanges[0].Start,
		})
	}

	// Gaps between available ranges
	for i := 0; i < len(g.availableRanges)-1; i++ {
		gapStart := g.availableRanges[i].End
		gapEnd := g.availableRanges[i+1].Start

		if gapStart < gapEnd {
			gaps = append(gaps, types.BlockRange{
				Start: gapStart,
				End:   gapEnd,
			})
		}
	}

	return gaps
}

// GetAvailableRanges returns all available ranges
func (g *GapTracker) GetAvailableRanges() []types.BlockRange {
	g.mu.RLock()
	defer g.mu.RUnlock()

	// Return a copy
	ranges := make([]types.BlockRange, len(g.availableRanges))
	copy(ranges, g.availableRanges)

	return ranges
}

// GetNextGap returns the next (largest) gap to fill
func (g *GapTracker) GetNextGap() *types.BlockRange {
	gaps := g.GetGaps()

	if len(gaps) == 0 {
		return nil
	}

	// Find the largest gap
	largestGap := gaps[0]
	largestSize := largestGap.End - largestGap.Start

	for _, gap := range gaps[1:] {
		size := gap.End - gap.Start
		if size > largestSize {
			largestGap = gap
			largestSize = size
		}
	}

	return &largestGap
}

// GetMinBlock returns the minimum tracked block
func (g *GapTracker) GetMinBlock() uint64 {
	g.mu.RLock()
	defer g.mu.RUnlock()

	return g.minBlock
}

// GetMaxBlock returns the maximum tracked block
func (g *GapTracker) GetMaxBlock() uint64 {
	g.mu.RLock()
	defer g.mu.RUnlock()

	return g.maxBlock
}

// GetTotalGapSize returns the total size of all gaps
func (g *GapTracker) GetTotalGapSize() uint64 {
	gaps := g.GetGaps()

	totalSize := uint64(0)
	for _, gap := range gaps {
		totalSize += gap.End - gap.Start
	}

	return totalSize
}

// GetCompletionPercentage returns the percentage of blocks available
func (g *GapTracker) GetCompletionPercentage() float64 {
	g.mu.RLock()
	defer g.mu.RUnlock()

	if g.maxBlock == 0 {
		return 0
	}

	totalRange := g.maxBlock - g.minBlock
	if totalRange == 0 {
		return 100.0
	}

	// Calculate total available
	totalAvailable := uint64(0)
	for _, r := range g.availableRanges {
		totalAvailable += r.End - r.Start
	}

	return (float64(totalAvailable) / float64(totalRange)) * 100.0
}

// mergeRanges merges overlapping or adjacent ranges
func (g *GapTracker) mergeRanges() {
	if len(g.availableRanges) <= 1 {
		return
	}

	// Sort ranges by start block
	sort.Slice(g.availableRanges, func(i, j int) bool {
		return g.availableRanges[i].Start < g.availableRanges[j].Start
	})

	// Merge overlapping/adjacent ranges
	merged := []types.BlockRange{g.availableRanges[0]}

	for i := 1; i < len(g.availableRanges); i++ {
		current := g.availableRanges[i]
		last := &merged[len(merged)-1]

		// Check if current range overlaps or is adjacent to last merged range
		if current.Start <= last.End {
			// Merge the ranges
			if current.End > last.End {
				last.End = current.End
			}
		} else {
			// No overlap, add as new range
			merged = append(merged, current)
		}
	}

	g.availableRanges = merged
}

// Reset clears all tracked ranges
func (g *GapTracker) Reset() {
	g.mu.Lock()
	defer g.mu.Unlock()

	g.availableRanges = []types.BlockRange{}
	g.minBlock = ^uint64(0)
	g.maxBlock = 0
}
