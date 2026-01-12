# Schedularr Roadmap

## Project Vision

Schedularr aims to be the most intuitive and powerful automation tool for Tunarr TV channel scheduling. By combining cron-based scheduling, intelligent content filtering, and series progression tracking, Schedularr eliminates manual programming while maintaining complete control over channel content.

### Core Principles

1. **Automation First**: Eliminate repetitive manual scheduling tasks
2. **Intelligent Scheduling**: Smart content rotation, conflict resolution, and gap filling
3. **Flexibility**: Support both filter-based and series-based programming
4. **Reliability**: Robust error handling, state persistence, and recovery
5. **Observability**: Comprehensive logging, metrics, and debugging tools

### Target Users

- Tunarr users managing multiple channels
- Content curators building thematic programming blocks
- Power users with large media libraries
- Home media server administrators

## Project Status Overview

| Phase       | Status              | Completion | Description                                 |
| :---------- | :------------------ | :--------- | :------------------------------------------ |
| **Phase 0** | 🟢 Mostly Complete | 85%        | Architecture alignment with athena patterns |
| **Phase 1** | ✅ Completed        | 100%       | Foundation & API verification               |
| **Phase 2** | ✅ Completed        | 100%       | Scheduler file architecture                 |
| **Phase 3** | 🟢 Mostly Complete | 90%        | Enhanced scheduling engine                  |
| **Phase 4** | 🟡 In Progress     | 40%        | Operational excellence & testing            |
| **Phase 5** | 🔴 Not Started     | 0%         | UX enhancements                             |

## Development Phases

### Phase 0: Architecture Alignment (85% Complete)

**Goal:** Adopt best practices from the athena project for long-term maintainability.

**Status:** 🟢 Mostly Complete

**Completed:**

- ✅ CUE schema validation for configurations
- ✅ CLI command restructuring (main.go + cmd/ pattern)
- ✅ Structured logging with log/slog
- ✅ Strict linting rules (.golangci.yml)
- ✅ Blocked deprecated packages (depguard)
- ✅ Makefile-based build system

**Remaining:**

- 🔄 Error handling standards (context.Context, retries)
- 🔄 Complete project documentation (ARCHITECTURE.md, SPECIFICATIONS.md, CONTRIBUTING.md)
- 🔄 E2E testing infrastructure

**Target Completion:** Q1 2026

---

### Phase 1: Foundation & API Verification (100% Complete)

**Goal:** Establish solid foundation with verified Tunarr integration.

**Status:** ✅ Completed

**Achievements:**

- ✅ Tunarr API research and endpoint verification
- ✅ Enhanced error handling with exponential backoff
- ✅ API response validation
- ✅ Integration tests with mock Tunarr instance

**Completed:** January 2026

---

### Phase 2: Scheduler File Architecture (100% Complete)

**Goal:** Separate scheduler configuration from app configuration.

**Status:** ✅ Completed

**Achievements:**

- ✅ Configuration separation (config.yaml vs scheduler.yaml)
- ✅ Priority-based scheduler file loading
- ✅ `scheduler init` command with CUE generation
- ✅ `scheduler validate` command
- ✅ `scheduler list` command
- ✅ Series-based scheduling data models
- ✅ SQLite state persistence

**Completed:** January 2026

---

### Phase 3: Enhanced Scheduling Engine (90% Complete)

**Goal:** Advanced scheduling features with intelligent content management.

**Status:** 🟢 Mostly Complete

**Completed:**

- ✅ Content fetching from Tunarr libraries
- ✅ Series episode fetching and validation
- ✅ Scheduling history tracking (7-day window)
- ✅ Smart content rotation (prevent recent repeats)
- ✅ Duplicate detection per channel
- ✅ Gap filling with filler content
- ✅ Priority-based conflict resolution
- ✅ Series state management (SQLite)

**Remaining:**

- 🔄 Strict timing mode (precise cron start times)
- 🔄 Timezone and DST handling
- 🔄 Series completion logging and auto-restart

**Target Completion:** Q1 2026

---

### Phase 4: Operational Excellence (40% Complete)

**Goal:** Production readiness with comprehensive testing and observability.

**Status:** 🟡 In Progress

**Completed:**

- ✅ Unit tests for core scheduling logic (67% coverage)
- ✅ Conflict resolution tests
- ✅ Gap filling tests
- ✅ History tracking tests
- ✅ Linting compliance (golangci-lint, gosec, govulncheck)
- ✅ Dockerization with multi-stage builds

**In Progress:**

- 🔄 Increase test coverage to >80%
- 🔄 Integration tests for full workflow
- 🔄 E2E tests with real Tunarr instance
- 🔄 Observability (Prometheus metrics, health checks)
- 🔄 Configuration management (env vars, hot-reload)

**Remaining:**

- ⏳ Structured logging migration (partial - engine only)
- ⏳ API documentation
- ⏳ Migration guides

**Target Completion:** Q1-Q2 2026

---

### Phase 5: UX Enhancements (0% Complete)

**Goal:** Improve user experience with better CLI and TUI features.

**Status:** 🔴 Not Started

**Planned Features:**

**CLI Improvements:**

- Better table output with colors and formatting
- Progress indicators for long operations
- Real-time schedule preview
- Dry run enhancements

**TUI Enhancements:**

- Real-time field validation
- Visual filter rule builder
- Confirmation dialogs for destructive actions
- Context-sensitive help system
- Keyboard shortcuts documentation

**Target Completion:** Q2 2026

---

## Feature Roadmap

### Near Term (Q1 2026)

1. **Complete Phase 0 Documentation**
   - Finish ARCHITECTURE.md, SPECIFICATIONS.md
   - Create CONTRIBUTING.md
   - Update CLAUDE.md with latest patterns

2. **Increase Test Coverage**
   - Add integration tests for full scheduling workflow
   - Create E2E test infrastructure with docker-compose
   - Achieve >80% code coverage

3. **Observability Improvements**
   - Add Prometheus metrics endpoint
   - Implement health check endpoints
   - Track scheduling operations and API latencies

### Mid Term (Q2 2026)

1. **Series Scheduling Enhancements**
   - Auto-restart completed series
   - Multi-season progression
   - Episode skip functionality
   - Series state backup/export

2. **Advanced Filtering**
   - Tag-based filtering
   - Actor/director filtering
   - Custom filter expressions
   - Filter templates

3. **Schedule Optimization**
   - Better gap filling algorithms
   - Content diversity scoring
   - Theme-based block suggestions
   - Schedule validation and warnings

### Long Term (Q3-Q4 2026)

1. **Multi-Channel Coordination**
   - Cross-channel scheduling
   - Channel groups
   - Shared content pools
   - Global priority rules

2. **Web UI (Optional)**
   - Visual schedule builder
   - Channel preview
   - Real-time schedule updates
   - Statistics dashboard

3. **Advanced Features**
   - Machine learning for content recommendations
   - Holiday/special event scheduling
   - Commercial break insertion
   - Viewer analytics integration

---

## Success Metrics

### Technical Metrics

- **Test Coverage:** >80% across all packages
- **Linting:** Zero errors, <10 acceptable warnings
- **Security:** Zero vulnerabilities (gosec, govulncheck)
- **Performance:** Schedule generation <1s for 100 blocks
- **Reliability:** >99.9% uptime in daemon mode

### User Metrics

- **Setup Time:** <5 minutes from install to first schedule
- **Schedule Accuracy:** >99% blocks scheduled without errors
- **Documentation Coverage:** 100% of features documented
- **Support Requests:** <1% of users need support for basic tasks

### Community Metrics

- **GitHub Stars:** 100+ (indicates usefulness)
- **Contributors:** 5+ active contributors
- **Issues Closed:** >90% within 30 days
- **Documentation Quality:** >95% positive feedback

---

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for details on:

- Code style and standards
- Commit message format
- Pull request process
- Code review checklist

---

## Release Strategy

### Versioning

Schedularr follows [Semantic Versioning 2.0.0](https://semver.org/):

- **MAJOR:** Breaking changes to configuration or API
- **MINOR:** New features, backwards-compatible
- **PATCH:** Bug fixes, backwards-compatible

### Release Cycle

- **Patch releases:** As needed for critical bugs
- **Minor releases:** Every 4-6 weeks
- **Major releases:** Planned around significant breaking changes

### Current Version

- **v0.4.0** (Current Development)
  - Phase 0-3 features
  - Core scheduling engine
  - Series-based scheduling
  - State persistence

### Upcoming Releases

- **v0.5.0** (Planned Q1 2026)
  - Complete Phase 4 features
  - E2E testing infrastructure
  - Prometheus metrics
  - Enhanced observability

- **v1.0.0** (Planned Q2 2026)
  - Production-ready release
  - All Phase 0-5 features complete
  - Comprehensive documentation
  - Stable API and configuration format

---

## Questions or Feedback?

- **Issues:** [GitHub Issues](https://github.com/geekxflood/schedularr/issues)
- **Discussions:** [GitHub Discussions](https://github.com/geekxflood/schedularr/discussions)
- **Documentation:** See [docs/](docs/) directory

---

*Last Updated: January 2026*
