package types

import (
	"testing"
	"time"

	_ "github.com/marcboeker/go-duckdb"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestStateAwareMarker_Muted(t *testing.T) {
	r := prometheus.NewRegistry()
	mock := gomock.NewController(t)

	appender := NewMockStateAppender(mock)
	marker := NewStateAwareMarker(r, appender)

	// No groups should be muted.
	timeIntervalNames, isMuted := marker.Muted("route1", "group1")
	require.False(t, isMuted)
	require.Empty(t, timeIntervalNames)

	// Mark the group as muted because it's the weekend.
	marker.SetMuted("route1", "group1", []string{"weekends"})
	timeIntervalNames, isMuted = marker.Muted("route1", "group1")
	require.True(t, isMuted)
	require.Equal(t, []string{"weekends"}, timeIntervalNames)

	// Other groups should not be marked as muted.
	timeIntervalNames, isMuted = marker.Muted("route1", "group2")
	require.False(t, isMuted)
	require.Empty(t, timeIntervalNames)

	// Other routes should not be marked as muted either.
	timeIntervalNames, isMuted = marker.Muted("route2", "group1")
	require.False(t, isMuted)
	require.Empty(t, timeIntervalNames)

	// The group is no longer muted.
	marker.SetMuted("route1", "group1", nil)
	timeIntervalNames, isMuted = marker.Muted("route1", "group1")
	require.False(t, isMuted)
	require.Empty(t, timeIntervalNames)
}

func TestStateAwareMarker_DeleteByGroupKey(t *testing.T) {
	r := prometheus.NewRegistry()
	mock := gomock.NewController(t)

	appender := NewMockStateAppender(mock)
	marker := NewStateAwareMarker(r, appender)

	// Mark the group and check that it is muted.
	marker.SetMuted("route1", "group1", []string{"weekends"})
	timeIntervalNames, isMuted := marker.Muted("route1", "group1")
	require.True(t, isMuted)
	require.Equal(t, []string{"weekends"}, timeIntervalNames)

	// Delete the markers for a different group key. The group should
	// still be muted.
	marker.DeleteByGroupKey("route1", "group2")
	timeIntervalNames, isMuted = marker.Muted("route1", "group1")
	require.True(t, isMuted)
	require.Equal(t, []string{"weekends"}, timeIntervalNames)

	// Delete the markers for the correct group key. The group should
	// no longer be muted.
	marker.DeleteByGroupKey("route1", "group1")
	timeIntervalNames, isMuted = marker.Muted("route1", "group1")
	require.False(t, isMuted)
	require.Empty(t, timeIntervalNames)
}

func TestStateAwareMarker_Count(t *testing.T) {
	r := prometheus.NewRegistry()
	mock := gomock.NewController(t)

	appender := NewMockStateAppender(mock)
	marker := NewStateAwareMarker(r, appender)

	now := time.Now()

	states := []AlertState{AlertStateSuppressed, AlertStateActive, AlertStateUnprocessed}
	countByState := func(state AlertState) int {
		return marker.Count(state)
	}

	countTotal := func() int {
		var count int
		for _, s := range states {
			count += countByState(s)
		}
		return count
	}

	require.Equal(t, 0, countTotal())

	a1 := model.Alert{
		StartsAt: now.Add(-2 * time.Minute),
		EndsAt:   now.Add(2 * time.Minute),
		Labels:   model.LabelSet{"test": "active"},
	}
	a2 := model.Alert{
		StartsAt: now.Add(-2 * time.Minute),
		EndsAt:   now.Add(2 * time.Minute),
		Labels:   model.LabelSet{"test": "suppressed"},
	}
	a3 := model.Alert{
		StartsAt: now.Add(-2 * time.Minute),
		EndsAt:   now.Add(-1 * time.Minute),
		Labels:   model.LabelSet{"test": "resolved"},
	}

	appender.EXPECT().Append(a1.Fingerprint(), AlertStateActive).Times(1)
	appender.EXPECT().Append(a3.Fingerprint(), AlertStateActive).Times(1)
	appender.EXPECT().Append(a2.Fingerprint(), AlertStateSuppressed).Times(1)
	appender.EXPECT().Append(a3.Fingerprint(), AlertStateSuppressed).Times(1)

	// Insert an active alert.
	marker.SetActiveOrSilenced(a1.Fingerprint(), 1, nil, nil)
	require.Equal(t, 1, countByState(AlertStateActive))
	require.Equal(t, 1, countTotal())

	// Insert a suppressed alert by silence.
	marker.SetActiveOrSilenced(a2.Fingerprint(), 1, []string{"1"}, nil)
	require.Equal(t, 1, countByState(AlertStateSuppressed))
	require.Equal(t, 2, countTotal())

	// Insert a resolved silenced alert - it'll count as suppressed.
	marker.SetActiveOrSilenced(a3.Fingerprint(), 1, []string{"2"}, nil)
	require.Equal(t, 2, countByState(AlertStateSuppressed))
	require.Equal(t, 3, countTotal())

	t.Logf("currentStatus of a3: %v", marker.Status(a3.Fingerprint()))

	// Insert a resolved alert - it'll count as active.
	marker.SetActiveOrSilenced(a3.Fingerprint(), 1, nil, nil)
	require.Equal(t, 2, countByState(AlertStateActive))
	require.Equal(t, 3, countTotal())

	t.Logf("currentStatus of a3: %v", marker.Status(a3.Fingerprint()))
}

// TestStateAwareMarker_Duplicate ensurs that when you call the same function twice, e.g.
// SetActiveOrSilenced and the state would be set to active twice, that we don't append the state
// active twice to storage.
func TestStateAwareMarker_Duplicate(t *testing.T) {
	r := prometheus.NewRegistry()
	mock := gomock.NewController(t)

	appender := NewMockStateAppender(mock)
	marker := NewStateAwareMarker(r, appender)

	now := time.Now()

	a1 := model.Alert{
		StartsAt: now.Add(-2 * time.Minute),
		EndsAt:   now.Add(2 * time.Minute),
		Labels:   model.LabelSet{"test": "active"},
	}
	a2 := model.Alert{
		StartsAt: now.Add(-2 * time.Minute),
		EndsAt:   now.Add(2 * time.Minute),
		Labels:   model.LabelSet{"test": "active2"},
	}

	appender.EXPECT().
		Append(a1.Fingerprint(), AlertStateActive).
		Do(func(any, any) {
			t.Log("Append Called")
		}).
		Times(1)
	appender.EXPECT().
		AppendInhibited(a1.Fingerprint(), []string{a2.Fingerprint().String()}).
		Do(func(any, any) {
			t.Log("AppendInhibited Called")
		}).
		Times(1)
	appender.EXPECT().
		Append(a1.Fingerprint(), AlertStateDeleted).
		Do(func(any, any) {
			t.Log("Append With Delete Called")
		}).
		Times(1)

	currentStatus := marker.Status(a1.Fingerprint())

	t.Logf("a1 fingerprint: %s", a1.Fingerprint())
	t.Logf("current status: %v", currentStatus)

	{
		{
			t.Log("setting active or silence")
			marker.SetActiveOrSilenced(a1.Fingerprint(), 1, nil, nil)

		}
		{
			currentStatus = marker.Status(a1.Fingerprint())
			t.Logf("current status: %v", currentStatus)

			t.Log("setting active or silence")
			marker.SetActiveOrSilenced(a1.Fingerprint(), 1, nil, nil)
		}
		{
			currentStatus = marker.Status(a1.Fingerprint())
			t.Logf("current status: %v", currentStatus)

			t.Log("setting inhibited")
			marker.SetInhibited(a1.Fingerprint(), a2.Fingerprint().String())
		}
		{
			currentStatus = marker.Status(a1.Fingerprint())
			t.Logf("current status: %v", currentStatus)

			t.Log("setting inhibited")
			marker.SetInhibited(a1.Fingerprint(), a2.Fingerprint().String())
		}
	}

	{

		{
			currentStatus = marker.Status(a1.Fingerprint())
			t.Logf("current status: %v", currentStatus)

			t.Log("setting active or silence")
			marker.SetActiveOrSilenced(a1.Fingerprint(), 1, nil, nil)
		}

		{
			currentStatus = marker.Status(a1.Fingerprint())
			t.Logf("current status: %v", currentStatus)

			t.Log("setting active or silence")
			marker.SetActiveOrSilenced(a1.Fingerprint(), 1, nil, nil)
		}
		{
			currentStatus = marker.Status(a1.Fingerprint())
			t.Logf("current status: %v", currentStatus)

			t.Log("setting inhibited")
			marker.SetInhibited(a1.Fingerprint(), a2.Fingerprint().String())
		}
		{
			currentStatus = marker.Status(a1.Fingerprint())
			t.Logf("current status: %v", currentStatus)

			t.Log("setting inhibited")
			marker.SetInhibited(a1.Fingerprint(), a2.Fingerprint().String())
		}
		{
			currentStatus = marker.Status(a1.Fingerprint())
			t.Logf("current status: %v", currentStatus)

			t.Log("setting deleted")
			marker.Delete(a1.Fingerprint())
		}
		{
			currentStatus = marker.Status(a1.Fingerprint())
			t.Logf("current status: %v", currentStatus)

			t.Log("setting deleted")
			marker.Delete(a1.Fingerprint())
		}
	}
}

func TestStateAwareMarker_SetInhibited(t *testing.T) {
	r := prometheus.NewRegistry()
	mock := gomock.NewController(t)
	appender := NewMockStateAppender(mock)
	marker := NewStateAwareMarker(r, appender)
	now := time.Now()

	a1 := model.Alert{
		StartsAt: now.Add(-2 * time.Minute),
		EndsAt:   now.Add(2 * time.Minute),
		Labels:   model.LabelSet{"test": "active"},
	}
	a2 := model.Alert{
		StartsAt: now.Add(-2 * time.Minute),
		EndsAt:   now.Add(2 * time.Minute),
		Labels:   model.LabelSet{"test": "suppressed"},
	}
	a3 := model.Alert{
		StartsAt: now.Add(-2 * time.Minute),
		EndsAt:   now.Add(-1 * time.Minute),
		Labels:   model.LabelSet{"test": "resolved"},
	}

	appender.EXPECT().AppendInhibited(a1.Fingerprint(), []string{a2.Fingerprint().String()})
	appender.EXPECT().AppendInhibited(a1.Fingerprint(), []string{a3.Fingerprint().String()})

	marker.SetInhibited(a1.Fingerprint(), a2.Fingerprint().String())
	marker.SetInhibited(a1.Fingerprint(), a3.Fingerprint().String())
}

func TestStateAwareMarker_SetInhibited_NoInhibition(t *testing.T) {
	r := prometheus.NewRegistry()
	mock := gomock.NewController(t)
	appender := NewMockStateAppender(mock)
	marker := NewStateAwareMarker(r, appender)
	now := time.Now()

	a1 := model.Alert{
		StartsAt: now.Add(-2 * time.Minute),
		EndsAt:   now.Add(2 * time.Minute),
		Labels:   model.LabelSet{"test": "active"},
	}
	a2 := model.Alert{
		StartsAt: now.Add(-2 * time.Minute),
		EndsAt:   now.Add(2 * time.Minute),
		Labels:   model.LabelSet{"test": "active2"},
	}

	appender.EXPECT().Append(a1.Fingerprint(), AlertStateSuppressed)
	appender.EXPECT().Append(a2.Fingerprint(), AlertStateActive)

	marker.SetActiveOrSilenced(a1.Fingerprint(), 1, []string{"1"}, nil)

	marker.SetInhibited(a1.Fingerprint())
	marker.SetInhibited(a2.Fingerprint())
}
