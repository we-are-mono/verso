// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package history

import (
	"context"
	"log"
	"time"
)

const (
	// tickEvery is how often sources are read. Rates come with their own
	// per-second readings; gauges are sampled six times a minute.
	tickEvery = 10 * time.Second
	// flushEvery is how often minute points reach the file: the most a power
	// cut can take.
	flushEvery = 15 * time.Minute
	// maintainEvery is how often periods roll up and expired points go.
	maintainEvery = time.Hour
)

// Source reports the readings taken since it was last asked, by series. A
// gauge answers one reading a series; a rate hands over its per-second ones.
type Source func(now time.Time) map[string][]float64

type tally struct {
	sum, peak float64
	n         int
}

// Collector folds readings into minute points for the store.
type Collector struct {
	store   *Store
	sources []Source

	minute time.Time
	open   map[string]*tally
}

func NewCollector(store *Store, sources ...Source) *Collector {
	return &Collector{store: store, sources: sources, open: map[string]*tally{}}
}

// Tick reads every source. A reading taken at now covers the seconds before
// it, so it counts toward the minute that holds now's previous instant; once
// that minute has moved on, the finished one goes to the store.
func (c *Collector) Tick(now time.Time) {
	minute := now.Add(-time.Nanosecond).Truncate(time.Minute)
	if !minute.Equal(c.minute) {
		c.close()
		c.minute = minute
	}
	for _, source := range c.sources {
		for series, values := range source(now) {
			if len(values) == 0 {
				continue
			}
			t := c.open[series]
			if t == nil {
				t = &tally{peak: values[0]}
				c.open[series] = t
			}
			for _, v := range values {
				t.sum += v
				t.n++
				t.peak = max(t.peak, v)
			}
		}
	}
}

func (c *Collector) close() {
	points := make([]Point, 0, len(c.open))
	for series, t := range c.open {
		if t.n > 0 {
			points = append(points, Point{Series: series, Time: c.minute, Average: t.sum / float64(t.n), Peak: t.peak})
		}
	}
	c.store.Add(points...)
	c.open = map[string]*tally{}
}

// Run ticks, flushes and maintains until ctx ends, then writes what it holds
// and closes the store.
func (c *Collector) Run(ctx context.Context) {
	tick := time.NewTicker(tickEvery)
	defer tick.Stop()
	flush := time.NewTicker(flushEvery)
	defer flush.Stop()
	maintain := time.NewTicker(maintainEvery)
	defer maintain.Stop()
	for {
		select {
		case <-ctx.Done():
			c.close()
			if err := c.store.Close(); err != nil {
				log.Printf("verso: history not saved: %v", err)
			}
			return
		case now := <-tick.C:
			c.Tick(now)
		case <-flush.C:
			if err := c.store.Flush(ctx); err != nil {
				log.Printf("verso: history not saved: %v", err)
			}
		case now := <-maintain.C:
			if err := c.store.Maintain(ctx, now); err != nil {
				log.Printf("verso: history not maintained: %v", err)
			}
		}
	}
}
