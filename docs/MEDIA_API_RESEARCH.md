# Media API Research

This document summarizes the external media APIs Schedularr uses for availability filtering (Radarr/Sonarr) and Live TV sync (Jellyfin).

## Radarr

- **Base URL:** `http://<radarr-host>:7878`
- **Auth:** `X-Api-Key` header
- **Endpoint used:**
  - `GET /api/v3/movie`
- **Fields used:**
  - `id`, `title`, `year`, `overview`, `runtime`, `genres`, `hasFile`, `monitored`, `certification`
- **Schedularr usage:**
  - Filters Tunarr movie programs to those with `hasFile == true`.

## Sonarr

- **Base URL:** `http://<sonarr-host>:8989`
- **Auth:** `X-Api-Key` header
- **Endpoints used:**
  - `GET /api/v3/series`
  - `GET /api/v3/episode?seriesId=<id>`
- **Fields used:**
  - Series: `id`, `title`, `year`, `overview`, `genres`, `runtime`
  - Episode: `id`, `seriesId`, `title`, `overview`, `seasonNumber`, `episodeNumber`, `runtime`, `hasFile`
- **Schedularr usage:**
  - Filters Tunarr episode programs to those with `hasFile == true`.

## Jellyfin

- **Base URL:** `http://<jellyfin-host>:8096`
- **Auth:** `X-Emby-Token` header (API key)
- **Endpoint used:**
  - `POST /LiveTv/RefreshGuide`
- **Schedularr usage:**
  - Optional Live TV guide refresh after schedule apply.
