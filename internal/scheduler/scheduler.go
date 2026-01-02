package scheduler

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/robfig/cron/v3"
)

// Job represents a scheduled task
type Job func(ctx context.Context) error

// Scheduler manages periodic tasks
type Scheduler struct {
	cron     *cron.Cron
	jobs     map[string]cron.EntryID
	timezone *time.Location
}

// New creates a new scheduler with the given timezone
func New(timezone string) (*Scheduler, error) {
	loc, err := time.LoadLocation(timezone)
	if err != nil {
		return nil, fmt.Errorf("invalid timezone %s: %w", timezone, err)
	}

	c := cron.New(cron.WithLocation(loc))

	return &Scheduler{
		cron:     c,
		jobs:     make(map[string]cron.EntryID),
		timezone: loc,
	}, nil
}

// AddJob adds a job with a cron schedule
// schedule format: "0 7 * * *" (at 7:00 AM daily)
func (s *Scheduler) AddJob(name, schedule string, job Job) error {
	entryID, err := s.cron.AddFunc(schedule, func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer cancel()

		log.Printf("[scheduler] Starting job: %s", name)
		start := time.Now()

		if err := job(ctx); err != nil {
			log.Printf("[scheduler] Job %s failed: %v", name, err)
		} else {
			log.Printf("[scheduler] Job %s completed in %v", name, time.Since(start))
		}
	})

	if err != nil {
		return fmt.Errorf("failed to schedule job %s: %w", name, err)
	}

	s.jobs[name] = entryID
	log.Printf("[scheduler] Added job: %s (schedule: %s)", name, schedule)

	return nil
}

// AddScrapeJob adds the scraping job
func (s *Scheduler) AddScrapeJob(intervalHours int, job Job) error {
	schedule := fmt.Sprintf("0 */%d * * *", intervalHours)
	return s.AddJob("scrape", schedule, job)
}

// AddDigestJob adds a digest job at a specific time
// timeStr format: "07:00" or "18:00"
func (s *Scheduler) AddDigestJob(name, timeStr string, job Job) error {
	// Parse time
	t, err := time.Parse("15:04", timeStr)
	if err != nil {
		return fmt.Errorf("invalid time format %s: %w", timeStr, err)
	}

	schedule := fmt.Sprintf("%d %d * * *", t.Minute(), t.Hour())
	return s.AddJob(name, schedule, job)
}

// RemoveJob removes a scheduled job
func (s *Scheduler) RemoveJob(name string) {
	if entryID, ok := s.jobs[name]; ok {
		s.cron.Remove(entryID)
		delete(s.jobs, name)
		log.Printf("[scheduler] Removed job: %s", name)
	}
}

// Start begins running scheduled jobs
func (s *Scheduler) Start() {
	log.Println("[scheduler] Starting scheduler")
	s.cron.Start()
}

// Stop halts the scheduler
func (s *Scheduler) Stop() context.Context {
	log.Println("[scheduler] Stopping scheduler")
	return s.cron.Stop()
}

// RunNow immediately executes a job (useful for testing)
func (s *Scheduler) RunNow(name string, job Job) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	log.Printf("[scheduler] Running job now: %s", name)
	return job(ctx)
}

// ListJobs returns info about scheduled jobs
func (s *Scheduler) ListJobs() []JobInfo {
	entries := s.cron.Entries()
	infos := make([]JobInfo, 0, len(entries))

	for name, entryID := range s.jobs {
		for _, entry := range entries {
			if entry.ID == entryID {
				infos = append(infos, JobInfo{
					Name:    name,
					NextRun: entry.Next,
					LastRun: entry.Prev,
				})
				break
			}
		}
	}

	return infos
}

// JobInfo contains information about a scheduled job
type JobInfo struct {
	Name    string
	NextRun time.Time
	LastRun time.Time
}
