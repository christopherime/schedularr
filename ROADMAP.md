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

| Phase       | Status       | Completion | Description                                 |
| :---------- | :----------- | :--------- | :------------------------------------------ |
| **Phase 0** | ✅ Completed | 100%       | Architecture alignment with athena patterns |
| **Phase 1** | ✅ Completed | 100%       | Foundation & API verification               |
| **Phase 2** | ✅ Completed | 100%       | Scheduler file architecture                 |
| **Phase 3** | ✅ Completed | 100%       | Enhanced scheduling engine                  |
| **Phase 4** | ✅ Completed | 100%       | Operational excellence & testing            |
| **Phase 5** | ✅ Completed | 100%       | UX enhancements                             |
| **Phase 8** | ✅ Completed | 100%       | Code reduction via external dependencies    |
| **Phase 9** | ✅ Completed | 100%       | Further code reduction via libraries        |

## Development Phases

### Phase 0: Architecture Alignment (100% Complete)

**Goal:** Adopt best practices from the athena project for long-term maintainability.

**Status:** ✅ Completed

**Achievements:**

- ✅ CUE schema validation for configurations
- ✅ CLI command restructuring (main.go + cmd/ pattern)
- ✅ Structured logging with log/slog
- ✅ Strict linting rules (.golangci.yml)
- ✅ Blocked deprecated packages (depguard)
- ✅ Makefile-based build system
- ✅ Error handling with context.Context and retries (avast/retry-go)
- ✅ Complete project documentation
- ✅ E2E testing infrastructure

**Completed:** January 2026

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

### Phase 3: Enhanced Scheduling Engine (100% Complete)

**Goal:** Advanced scheduling features with intelligent content management.

**Status:** ✅ Completed

**Achievements:**

- ✅ Content fetching from Tunarr libraries
- ✅ Series episode fetching and validation
- ✅ Scheduling history tracking (configurable retention)
- ✅ Smart content rotation (prevent recent repeats)
- ✅ Duplicate detection per channel
- ✅ Gap filling with filler content
- ✅ Priority-based conflict resolution
- ✅ Series state management (SQLite with golang-migrate)
- ✅ Timezone support via configuration
- ✅ Series completion tracking with run count

**Completed:** January 2026

---

### Phase 4: Operational Excellence (100% Complete)

**Goal:** Production readiness with comprehensive testing and observability.

**Status:** ✅ Completed

**Achievements:**

- ✅ Test coverage >85% for most packages (100% for cache, jellyfin)
- ✅ Integration tests for full scheduling workflow
- ✅ E2E tests for series scheduling with state persistence
- ✅ Conflict resolution and mixed block type tests
- ✅ Linting compliance (golangci-lint, gosec, govulncheck)
- ✅ Prometheus metrics for scheduling engine and Tunarr API client
- ✅ Automatic schedule history cleanup (configurable retention)
- ✅ Background cleaner component for daemon mode
- ✅ Structured logging with slog throughout

**Completed:** January 2026

---

### Phase 5: UX Enhancements (100% Complete)

**Goal:** Improve user experience with better CLI and TUI features.

**Status:** ✅ Completed

**CLI Achievements:**

- ✅ Better table output with go-pretty (colors, auto-sizing)
- ✅ Schedule preview with `generate --dry-run`
- ✅ Series state management commands (list, set, reset, export, import, backup)

**TUI Achievements:**

- ✅ Full block CRUD operations (create, edit, delete, duplicate)
- ✅ Real-time field validation with charmbracelet/huh forms
- ✅ Block priority adjustment (+/- keys)
- ✅ Save to disk functionality (ctrl+s)
- ✅ 24-hour schedule timeline view (t key)
- ✅ Series state viewer with edit capability (s/e keys)
- ✅ Conflict detection and highlighting
- ✅ Unsaved changes indicator

**Completed:** January 2026

---

### Phase 8: Code Reduction via External Dependencies (100% Complete)

**Goal:** Reduce codebase size while maintaining functionality.

**Status:** ✅ Completed

**Achievements:**

- ✅ TUI form handling with charmbracelet/huh (~100 lines saved)
- ✅ In-memory cache with patrickmn/go-cache (~30 lines saved)
- ✅ Database layer with jmoiron/sqlx (~60 lines saved)
- ✅ Cron builder extraction to reusable package (~170 lines saved)
- ✅ HTTP client middleware simplification (~10 lines saved)

**Total Savings:** ~370 lines of custom code

**Completed:** January 2026

---

### Phase 9: Further Code Reduction via Libraries (100% Complete)

**Goal:** Further reduce custom code with well-maintained libraries.

**Status:** ✅ Completed

**Achievements:**

- ✅ CLI table output with jedib0t/go-pretty (~60 lines saved)
- ✅ Database migrations with golang-migrate (~35 lines saved)
- ✅ Retry logic with avast/retry-go (~8 lines saved)

**Total Savings:** ~103 lines of custom code

**Completed:** January 2026

---

## Feature Roadmap

### Completed (Q1 2026) ✅

1. **Documentation**
   - ✅ ARCHITECTURE.md, SPECIFICATIONS.md complete
   - ✅ CONTRIBUTING.md created
   - ✅ CLAUDE.md updated with latest patterns

2. **Test Coverage**
   - ✅ Integration tests for full scheduling workflow
   - ✅ E2E test infrastructure
   - ✅ >85% code coverage achieved

3. **Observability**
   - ✅ Prometheus metrics for scheduling and API calls
   - ✅ Structured logging with slog
   - ✅ Operation tracking

4. **Series Scheduling**
   - ✅ Multi-season progression
   - ✅ Series state backup/export/import
   - ✅ Run count tracking

### Future Enhancements (Backlog)

1. **Advanced Filtering**
   - Tag-based filtering
   - Actor/director filtering
   - Custom filter expressions
   - Filter templates

2. **Schedule Optimization**
   - Better gap filling algorithms
   - Content diversity scoring
   - Theme-based block suggestions

3. **Multi-Channel Coordination**
   - Cross-channel scheduling
   - Channel groups
   - Shared content pools

4. **Web UI (Optional)**
   - Visual schedule builder
   - Channel preview
   - Statistics dashboard

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

- **v1.0.0** (January 2026)
  - All Phase 0-9 features complete
  - Production-ready release
  - Comprehensive test coverage (>85%)
  - Full documentation
  - Stable API and configuration format

### Recent Releases

- **v0.5.0** - Phase 4 & 5 completion (testing, UX enhancements)
- **v0.4.0** - Phase 3 completion (enhanced scheduling engine)
- **v0.3.0** - Phase 2 completion (scheduler file architecture)

### Future Releases

- **v1.1.0** (Planned)
  - Advanced filtering features
  - Schedule optimization improvements
  - Additional backlog items as prioritized

---

## Questions or Feedback?

- **Issues:** [GitHub Issues](https://github.com/geekxflood/schedularr/issues)
- **Discussions:** [GitHub Discussions](https://github.com/geekxflood/schedularr/discussions)
- **Documentation:** See [docs/](docs/) directory

---

*Last Updated: January 2026*
