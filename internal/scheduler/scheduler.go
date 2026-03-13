package scheduler

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/robfig/cron/v3"

	"github.com/alokemajumder/AegisClaw/internal/database/repository"
	"github.com/alokemajumder/AegisClaw/internal/models"
	natspkg "github.com/alokemajumder/AegisClaw/internal/nats"
)

// Scheduler manages cron-based engagement execution.
type Scheduler struct {
	cron        *cron.Cron
	engagements *repository.EngagementRepo
	publisher   *natspkg.Publisher
	logger      *slog.Logger

	mu       sync.Mutex
	entryMap map[uuid.UUID]cron.EntryID // engagement ID → cron entry
}

// New creates a new Scheduler.
func New(engagements *repository.EngagementRepo, publisher *natspkg.Publisher, logger *slog.Logger) *Scheduler {
	return &Scheduler{
		cron:        cron.New(cron.WithSeconds()),
		engagements: engagements,
		publisher:   publisher,
		logger:      logger,
		entryMap:    make(map[uuid.UUID]cron.EntryID),
	}
}

// Start loads active engagements and begins the cron scheduler.
func (s *Scheduler) Start(ctx context.Context) error {
	if err := s.loadEngagements(ctx); err != nil {
		s.logger.Error("failed to load engagements", "error", err)
	}

	s.cron.Start()
	s.logger.Info("scheduler started", "jobs", len(s.entryMap))

	// Periodically reload engagements to pick up new/changed ones.
	go s.reloadLoop(ctx)

	return nil
}

// Stop gracefully stops the scheduler.
func (s *Scheduler) Stop() {
	ctx := s.cron.Stop()
	<-ctx.Done()
	s.logger.Info("scheduler stopped")
}

func (s *Scheduler) loadEngagements(ctx context.Context) error {
	active, err := s.engagements.ListActive(ctx)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for _, eng := range active {
		if eng.ScheduleCron == nil || *eng.ScheduleCron == "" {
			continue
		}
		if _, exists := s.entryMap[eng.ID]; exists {
			continue
		}
		s.addJob(eng)
	}

	return nil
}

func (s *Scheduler) addJob(eng models.Engagement) {
	schedule := *eng.ScheduleCron
	engID := eng.ID
	orgID := eng.OrgID

	entryID, err := s.cron.AddFunc(schedule, func() {
		s.triggerRun(engID, orgID)
	})
	if err != nil {
		s.logger.Error("failed to add cron job", "engagement_id", engID, "schedule", schedule, "error", err)
		return
	}

	s.entryMap[engID] = entryID
	s.logger.Info("scheduled engagement", "engagement_id", engID, "schedule", schedule)
}

func (s *Scheduler) triggerRun(engagementID, orgID uuid.UUID) {
	if IsInBlackout(time.Now()) {
		s.logger.Info("skipping run due to blackout window", "engagement_id", engagementID)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	msg := natspkg.RunTriggerMsg{
		EngagementID: engagementID,
		TriggeredBy:  "scheduler",
	}

	if err := s.publisher.Publish(ctx, natspkg.SubjectRunTrigger, orgID, msg); err != nil {
		s.logger.Error("failed to publish run trigger", "engagement_id", engagementID, "error", err)
		return
	}

	s.logger.Info("triggered scheduled run", "engagement_id", engagementID)
}

func (s *Scheduler) reloadLoop(ctx context.Context) {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := s.loadEngagements(ctx); err != nil {
				s.logger.Error("failed to reload engagements", "error", err)
			}
		}
	}
}

// RemoveEngagement removes a scheduled engagement.
func (s *Scheduler) RemoveEngagement(engID uuid.UUID) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if entryID, exists := s.entryMap[engID]; exists {
		s.cron.Remove(entryID)
		delete(s.entryMap, engID)
		s.logger.Info("removed scheduled engagement", "engagement_id", engID)
	}
}
