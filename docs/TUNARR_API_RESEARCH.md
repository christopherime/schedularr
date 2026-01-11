# Tunarr API Research

## Overview

This document tracks research findings about the Tunarr API to ensure proper integration with Schedularr.

**Tunarr Repository**: <https://github.com/chrisbenincasa/tunarr>
**Documentation**: <https://tunarr.com>
**API Docs**: <https://tunarr.com/api-docs.html> (Scalar-based, requires JavaScript)

## Technology Stack

- **Backend**: Node.js 22+ (TypeScript)
- **Frontend**: React/Vite
- **API**: REST API with OpenAPI/Swagger documentation
- **Database**: SQLite (based on project structure)

## Known API Endpoints

Based on documentation and README analysis:

### Channels

- `GET /api/channels` - List all channels
  - **Status**: ✅ Implemented in schedularr
  - **Response**: Array of Channel objects
  - **Fields**: id, number, name, icon, groupTitle, enabled

### Programs/Content

- `GET /api/programs` - Fetch programs/media (NEEDS VERIFICATION)
  - **Status**: ⚠️ Placeholder implementation
  - **Note**: Endpoint may be different in actual API
  - **Alternative endpoints to investigate**:
    - `/api/channels/{id}/programs`
    - `/api/filler-lists`
    - `/api/programs/search`
    - Library-specific endpoints (Plex/Jellyfin/Emby)

### Schedule Management

- `POST /api/channels/{id}/schedule` - Update channel programming (NEEDS VERIFICATION)
  - **Status**: ⚠️ Placeholder implementation
  - **Payload**: Array of Program objects or IDs
  - **Note**: Need to confirm exact payload structure

### Other Endpoints (from API docs search)

- `/api/channels/{id}/shows` - Channel shows
- `/api/channels/{id}/artists` - Channel artists
- `/api/channels/{id}/transcode_config` - Transcoding configuration
- `/api/xmltv.xml` - XMLTV EPG data

## Authentication

- **API Key**: Optional, configured via `api_key` parameter
- **Header**: Likely `X-API-Key` or similar (NEEDS VERIFICATION)
- **Note**: Many Tunarr instances run without authentication in local networks

## Data Models

### Channel

```typescript
{
  id: string
  number: number
  name: string
  icon: string
  groupTitle: string
  enabled: boolean
}
```

### Program

```typescript
{
  id: string
  title: string
  year: number
  summary: string
  duration: number  // milliseconds
  rating: string
  genres: string[]
  type: string  // "movie", "episode", "track"
  showTitle?: string  // for episodes
  season?: number
  episode?: number
}
```

## Integration Points for Schedularr

### Phase 1: Verification Tasks

1. **Test Channel Endpoint**
   - Verify `/api/channels` response structure
   - Confirm authentication requirements
   - Test with real Tunarr instance

2. **Identify Content Fetching Endpoint**
   - Determine correct endpoint for fetching available programs
   - Test different endpoints:
     - `/api/programs`
     - `/api/channels/{id}/programs`
     - `/api/programs/search`
   - Understand filtering/pagination options

3. **Verify Schedule Update Endpoint**
   - Confirm endpoint for updating channel schedules
   - Understand payload structure
   - Test schedule update workflow

4. **API Key Authentication**
   - Test with and without API key
   - Determine correct header format
   - Handle authentication errors

## Next Steps

- [ ] Set up local Tunarr instance for testing
- [ ] Create integration tests against real API
- [ ] Document actual API behavior
- [ ] Update client implementation based on findings
- [ ] Add retry logic and error handling
- [ ] Implement proper response validation

## References

- Tunarr GitHub: <https://github.com/chrisbenincasa/tunarr>
- Tunarr Docs: <https://tunarr.com>
- Tunarr Discord: <https://discord.gg/svgSBYkEK5>
