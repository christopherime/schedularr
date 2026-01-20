package schedule

import (
	"container/list"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	// DefaultCacheSize is the default number of weeks to keep in memory.
	DefaultCacheSize = 10
)

// WeekCache is a thread-safe LRU cache for week schedules.
type WeekCache struct {
	capacity int
	cache    map[string]*list.Element
	lru      *list.List
	mu       sync.RWMutex
}

type cacheEntry struct {
	weekID   string
	schedule *WeekSchedule
}

// NewWeekCache creates a new LRU cache with the specified capacity.
func NewWeekCache(capacity int) *WeekCache {
	if capacity <= 0 {
		capacity = DefaultCacheSize
	}
	return &WeekCache{
		capacity: capacity,
		cache:    make(map[string]*list.Element),
		lru:      list.New(),
	}
}

// Get retrieves a week schedule from the cache.
func (c *WeekCache) Get(weekID string) (*WeekSchedule, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if elem, ok := c.cache[weekID]; ok {
		c.lru.MoveToFront(elem)
		return elem.Value.(*cacheEntry).schedule, true
	}
	return nil, false
}

// Put adds or updates a week schedule in the cache.
func (c *WeekCache) Put(weekID string, schedule *WeekSchedule) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if elem, ok := c.cache[weekID]; ok {
		c.lru.MoveToFront(elem)
		elem.Value.(*cacheEntry).schedule = schedule
		return
	}

	// Evict oldest if at capacity
	if c.lru.Len() >= c.capacity {
		oldest := c.lru.Back()
		if oldest != nil {
			c.lru.Remove(oldest)
			delete(c.cache, oldest.Value.(*cacheEntry).weekID)
		}
	}

	// Add new entry
	entry := &cacheEntry{weekID: weekID, schedule: schedule}
	elem := c.lru.PushFront(entry)
	c.cache[weekID] = elem
}

// Remove removes a week schedule from the cache.
func (c *WeekCache) Remove(weekID string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if elem, ok := c.cache[weekID]; ok {
		c.lru.Remove(elem)
		delete(c.cache, weekID)
	}
}

// Clear removes all entries from the cache.
func (c *WeekCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.cache = make(map[string]*list.Element)
	c.lru.Init()
}

// Manager handles week-based schedule file operations with lazy loading.
type Manager struct {
	schedulesDir string
	timezone     *time.Location
	cache        *WeekCache
	logger       *slog.Logger
	mu           sync.RWMutex
}

// NewManager creates a new schedule manager.
func NewManager(dir string, tz *time.Location, logger *slog.Logger) *Manager {
	if tz == nil {
		tz = time.Local
	}
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	}

	return &Manager{
		schedulesDir: dir,
		timezone:     tz,
		cache:        NewWeekCache(DefaultCacheSize),
		logger:       logger,
	}
}

// weekFilePath returns the file path for a given week ID.
func (m *Manager) weekFilePath(weekID string) string {
	return filepath.Join(m.schedulesDir, weekID+".yaml")
}

// GetWeek retrieves a week schedule, loading from disk if not cached.
// Returns an empty schedule if the file doesn't exist.
func (m *Manager) GetWeek(weekID string) (*WeekSchedule, error) {
	// Check cache first
	if schedule, ok := m.cache.Get(weekID); ok {
		m.logger.Debug("cache hit", "week_id", weekID)
		return schedule, nil
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Double-check cache after acquiring lock
	if schedule, ok := m.cache.Get(weekID); ok {
		return schedule, nil
	}

	// Load from disk
	schedule, err := m.loadFromDisk(weekID)
	if err != nil {
		return nil, err
	}

	// Cache the result
	m.cache.Put(weekID, schedule)
	return schedule, nil
}

// GetWeekForTime retrieves the week schedule containing the given time.
func (m *Manager) GetWeekForTime(t time.Time) (*WeekSchedule, error) {
	weekID := WeekID(t)
	return m.GetWeek(weekID)
}

// loadFromDisk loads a week schedule from disk, creating an empty one if not found.
func (m *Manager) loadFromDisk(weekID string) (*WeekSchedule, error) {
	filePath := m.weekFilePath(weekID)

	data, err := os.ReadFile(filePath)
	if os.IsNotExist(err) {
		// File doesn't exist, return empty schedule
		m.logger.Debug("creating empty schedule", "week_id", weekID)
		start, err := WeekStartFromID(weekID, m.timezone)
		if err != nil {
			return nil, fmt.Errorf("invalid week ID %s: %w", weekID, err)
		}
		_, end := WeekBounds(start, m.timezone)
		return NewWeekSchedule(weekID, start, end, m.timezone), nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to read schedule file %s: %w", filePath, err)
	}

	var schedule WeekSchedule
	if err := yaml.Unmarshal(data, &schedule); err != nil {
		return nil, fmt.Errorf("failed to parse schedule file %s: %w", filePath, err)
	}

	m.logger.Debug("loaded schedule from disk", "week_id", weekID, "programs", schedule.TotalProgramCount())
	return &schedule, nil
}

// SaveWeek persists a week schedule to disk and updates the cache.
func (m *Manager) SaveWeek(schedule *WeekSchedule) error {
	if schedule == nil {
		return fmt.Errorf("cannot save nil schedule")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	schedule.ModifiedAt = time.Now()

	data, err := yaml.Marshal(schedule)
	if err != nil {
		return fmt.Errorf("failed to marshal schedule: %w", err)
	}

	filePath := m.weekFilePath(schedule.WeekID)

	// Ensure directory exists
	if err := os.MkdirAll(m.schedulesDir, 0750); err != nil {
		return fmt.Errorf("failed to create schedules directory: %w", err)
	}

	// Write file with secure permissions
	// #nosec G306 - Schedule files should be readable by owner only
	if err := os.WriteFile(filePath, data, 0600); err != nil {
		return fmt.Errorf("failed to write schedule file: %w", err)
	}

	// Update cache
	m.cache.Put(schedule.WeekID, schedule)

	m.logger.Debug("saved schedule to disk", "week_id", schedule.WeekID, "programs", schedule.TotalProgramCount())
	return nil
}

// ListWeeks returns a sorted list of all week IDs with schedule files.
func (m *Manager) ListWeeks() ([]string, error) {
	entries, err := os.ReadDir(m.schedulesDir)
	if os.IsNotExist(err) {
		return []string{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to read schedules directory: %w", err)
	}

	var weeks []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasSuffix(name, ".yaml") && strings.Contains(name, "-W") {
			weekID := strings.TrimSuffix(name, ".yaml")
			// Validate the week ID format
			if _, _, err := ParseWeekID(weekID); err == nil {
				weeks = append(weeks, weekID)
			}
		}
	}

	sort.Strings(weeks)
	return weeks, nil
}

// DeleteWeek removes a week schedule file and clears it from cache.
func (m *Manager) DeleteWeek(weekID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.cache.Remove(weekID)

	filePath := m.weekFilePath(weekID)
	if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete schedule file: %w", err)
	}

	m.logger.Debug("deleted schedule", "week_id", weekID)
	return nil
}

// GetWeeksInRange returns all week IDs within a date range.
func (m *Manager) GetWeeksInRange(start, end time.Time) []string {
	var weeks []string
	current := start

	for !current.After(end) {
		weeks = append(weeks, WeekID(current))
		// Move to next week
		current = current.AddDate(0, 0, 7)
	}

	// Deduplicate in case start and end are in the same week
	seen := make(map[string]bool)
	unique := make([]string, 0, len(weeks))
	for _, w := range weeks {
		if !seen[w] {
			seen[w] = true
			unique = append(unique, w)
		}
	}

	return unique
}

// Timezone returns the manager's configured timezone.
func (m *Manager) Timezone() *time.Location {
	return m.timezone
}

// SchedulesDir returns the schedules directory path.
func (m *Manager) SchedulesDir() string {
	return m.schedulesDir
}

// ClearCache clears the in-memory cache, forcing reload from disk on next access.
func (m *Manager) ClearCache() {
	m.cache.Clear()
}
