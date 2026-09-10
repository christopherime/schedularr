package api

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/christopherime/schedularr/internal/api/gen"
)

// A cron that is airing right now: it starts on the current hour and runs
// for two, so the occurrence contains now whenever the test runs.
func onAirCron(t *testing.T) string {
	t.Helper()
	return fmt.Sprintf("0 %d * * *", time.Now().Hour())
}

// A cron that is definitely NOT airing: twelve hours away.
func offAirCron(t *testing.T) string {
	t.Helper()
	return fmt.Sprintf("0 %d * * *", (time.Now().Hour()+12)%24)
}

func seedBlockWithCron(t *testing.T, h http.Handler, name, cron string) gen.BlockRecord {
	t.Helper()
	body := filterBlockWrite(name, cron)
	body.Spec.Duration = 120
	w := doRequest(t, h, http.MethodPost, "/blocks", body)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	return decodeBlockRecord(t, w)
}

func TestDeleteBlock_RefusesWhileTheBlockIsOnAir(t *testing.T) {
	h := newTestServer(t)
	rec := seedBlockWithCron(t, h, "on-air-delete", onAirCron(t))

	w := doRequest(t, h, http.MethodDelete, "/blocks/"+rec.Id, nil)
	require.Equal(t, http.StatusConflict, w.Code, w.Body.String())

	p := decodeProblem(t, w)
	assert.Equal(t, "block is on air", p.Title)
	assert.Contains(t, p.Detail, "on-air-delete", "the refusal should name the block")
	assert.Regexp(t, `\d\d:\d\d`, p.Detail, "the refusal should name when it becomes safe")

	// And the block is still there.
	get := doRequest(t, h, http.MethodGet, "/blocks/"+rec.Id, nil)
	assert.Equal(t, http.StatusOK, get.Code)
}

func TestDeleteBlock_AllowsWhenTheBlockIsNotOnAir(t *testing.T) {
	h := newTestServer(t)
	rec := seedBlockWithCron(t, h, "off-air-delete", offAirCron(t))

	w := doRequest(t, h, http.MethodDelete, "/blocks/"+rec.Id, nil)
	assert.Equal(t, http.StatusNoContent, w.Code, w.Body.String())
}

// An already-disabled block generates no shell at the next apply, so
// deleting it cannot cut anything off -- and refusing would block the
// operator from tidying up something they already switched off.
func TestDeleteBlock_AllowsAnAlreadyDisabledBlockEvenOnItsAirtime(t *testing.T) {
	h := newTestServer(t)
	// Created disabled: an on-air block cannot be switched off through
	// PATCH (that is the guard working), so the precondition has to be set
	// at creation rather than after the fact.
	body := filterBlockWrite("disabled-delete", onAirCron(t))
	body.Spec.Duration = 120
	disabled := false
	body.Enabled = &disabled
	created := doRequest(t, h, http.MethodPost, "/blocks", body)
	require.Equal(t, http.StatusCreated, created.Code, created.Body.String())
	rec := decodeBlockRecord(t, created)

	w := doRequest(t, h, http.MethodDelete, "/blocks/"+rec.Id, nil)
	assert.Equal(t, http.StatusNoContent, w.Code, w.Body.String())
}

func TestPatchBlock_RefusesToDisableAnOnAirBlock(t *testing.T) {
	h := newTestServer(t)
	rec := seedBlockWithCron(t, h, "on-air-disable", onAirCron(t))

	w := doRequest(t, h, http.MethodPatch, "/blocks/"+rec.Id, map[string]any{"enabled": false})
	require.Equal(t, http.StatusConflict, w.Code, w.Body.String())
	assert.Equal(t, "block is on air", decodeProblem(t, w).Title)
}

func TestPatchBlock_RefusesToDarkenAnOnAirBlock(t *testing.T) {
	h := newTestServer(t)
	rec := seedBlockWithCron(t, h, "on-air-dark", onAirCron(t))

	until := time.Now().Add(48 * time.Hour).UTC().Format(time.RFC3339)
	w := doRequest(t, h, http.MethodPatch, "/blocks/"+rec.Id, map[string]any{"disabled_until": until})
	require.Equal(t, http.StatusConflict, w.Code, w.Body.String())
}

// Switching a block ON, or clearing its dark window, ADDS to the next
// apply's lineup. Nothing on air can be cut off by that, so the guard must
// not fire -- otherwise an operator could never re-enable a block during
// its own airtime.
func TestPatchBlock_AllowsPuttingAnOnAirBlockBackOn(t *testing.T) {
	h := newTestServer(t)
	rec := seedBlockWithCron(t, h, "re-enable", onAirCron(t))

	off := doRequest(t, h, http.MethodPatch, "/blocks/"+rec.Id, map[string]any{"enabled": false})
	require.Equal(t, http.StatusConflict, off.Code, "precondition: it is on air")

	// Clearing a dark window on a block that has none is still an "on"
	// patch and must be allowed.
	w := doRequest(t, h, http.MethodPatch, "/blocks/"+rec.Id, map[string]any{"disabled_until": nil})
	assert.Equal(t, http.StatusOK, w.Code, w.Body.String())
}

// PUT is the fourth path, and the least obvious one: moving a block's
// cron, channel or length makes the next apply plan a different
// occurrence, so the shell for the one on air is never injected and its
// channel re-anchors at now.
func TestUpdateBlock_RefusesToMoveAnOnAirBlock(t *testing.T) {
	h := newTestServer(t)
	rec := seedBlockWithCron(t, h, "on-air-move", onAirCron(t))

	moved := filterBlockWrite("on-air-move", offAirCron(t))
	moved.Spec.Duration = 120
	w := putBlock(t, h, rec.Id, moved)
	require.Equal(t, http.StatusConflict, w.Code, w.Body.String())
	assert.Equal(t, "block is on air", decodeProblem(t, w).Title)
}

// ...but a FILTER edit keeps the occurrence exactly where it is, so
// refusing it would cost the operator hours of editing time for no
// protection whatever.
func TestUpdateBlock_AllowsAFilterEditWhileOnAir(t *testing.T) {
	h := newTestServer(t)
	cron := onAirCron(t)
	rec := seedBlockWithCron(t, h, "on-air-filter", cron)

	same := filterBlockWrite("on-air-filter", cron)
	same.Spec.Duration = 120
	genres := []string{"Animation"}
	same.Spec.Filter = &gen.Filter{Genres: &genres}

	w := putBlock(t, h, rec.Id, same)
	assert.Equal(t, http.StatusOK, w.Code, w.Body.String())
}
