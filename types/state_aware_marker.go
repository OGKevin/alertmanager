//go:generate mockgen -package types -destination appender_mock_test.go . StateAppender
package types

import (
	"slices"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"

	_ "github.com/marcboeker/go-duckdb"
)

type marker interface {
	AlertMarker
	GroupMarker
}

// FlushableMarker is a marker that you should flush before you attempt to read state data.
// You must flush to ensure that all the data is written to disk or committed to the DB.
type FlushableMarker interface {
	marker
	Flush() error
}

// StateAppender appends the passed state to the underlying storage system.
// It is stupid, as in, if you call any of the methods, the state will be written.
// It's the caller's resopnsibility to ensure that no duplicate calls happen.
type StateAppender interface {
	// Append appends the state for the given fingerprint.
	Append(fingerprint model.Fingerprint, state AlertState)
	// AppendInhibited is the same as Append, with the only difference being that it takes extra
	// arguments to record who inhibited the alert.
	// Since the method is specific, you don't have to pass the state.
	AppendInhibited(fingerprint model.Fingerprint, inhibitedBy []string)
	// Close closes the underlying handler to the storage system.
	// Depending on the implementation, this could be a noop.
	Close() error
	// Flush some implementations will keep the data in memory until a certain condition is true or
	// flush has been called. This method is here to expose this functionablility if the caller
	// needs to read the appended data from storage.
	Flush() error
}

func NewStateAwareMarker(
	r prometheus.Registerer,
	appender StateAppender,
) *StateAwareMarker {
	return &StateAwareMarker{
		marker:   NewMarker(r),
		appender: appender,
	}
}

// StateAwareMarker implements FlushableMarker
// It reuses MemMarker and therefore doesn't re-implement any functionality.
// All it does is call StateAppender with the correct state, after passing the call to MemMarker.
// MemMarker's implementation contains noops and default actions depending on the existence or lack
// thereof of the passed alert. So we need to perform checks to see if the call to MemMarker was
// noop or not and append the state accordingly.
//
// Next to this, we also need to ensure that we don't append the same state more then once.
// There is a test case that cover this: TestStateAwareMarker_Duplicate
type StateAwareMarker struct {
	marker   marker
	appender StateAppender
}

func (s *StateAwareMarker) SetActiveOrSilenced(
	alert model.Fingerprint,
	version int,
	activeSilenceIDs,
	pendingSilenceIDs []string,
) {
	currStatus := s.marker.Status(alert)

	s.marker.SetActiveOrSilenced(alert, version, activeSilenceIDs, pendingSilenceIDs)

	// There is a chance that marker marks it as active, so we need to check.
	if _, _, _, isSilenced := s.marker.Silenced(alert); isSilenced {
		if currStatus.State != AlertStateSuppressed {
			s.appender.Append(alert, AlertStateSuppressed)
		}

		return
	}

	if currStatus.State == AlertStateSuppressed && len(currStatus.InhibitedBy) > 0 {
		return
	}

	if currStatus.State == AlertStateActive {
		return
	}

	s.appender.Append(alert, AlertStateActive)
}

func (s *StateAwareMarker) SetInhibited(alert model.Fingerprint, alertIDs ...string) {
	currStatus := s.marker.Status(alert)

	s.marker.SetInhibited(alert, alertIDs...)

	if by, isInhibited := s.marker.Inhibited(alert); isInhibited {
		if currStatus.State != AlertStateSuppressed {
			s.appender.AppendInhibited(alert, by)

			return
		}

		if !slices.Equal(currStatus.InhibitedBy, alertIDs) {
			s.appender.AppendInhibited(alert, by)
		}

		return
	}

	if currStatus.State == AlertStateSuppressed || currStatus.State == AlertStateActive {
		return
	}

	s.appender.Append(alert, AlertStateActive)
}

func (s *StateAwareMarker) Count(states ...AlertState) int {
	return s.marker.Count(states...)
}

func (s *StateAwareMarker) Status(alert model.Fingerprint) AlertStatus {
	return s.marker.Status(alert)
}

func (s *StateAwareMarker) Delete(alert model.Fingerprint) {
	currStatus := s.marker.Status(alert)

	s.marker.Delete(alert)

	if currStatus.State == AlertStateUnprocessed {
		return
	}

	s.appender.Append(alert, AlertStateDeleted)
}

func (s *StateAwareMarker) Unprocessed(alert model.Fingerprint) bool {
	return s.marker.Unprocessed(alert)
}

func (s *StateAwareMarker) Active(alert model.Fingerprint) bool {
	return s.marker.Active(alert)
}

func (s *StateAwareMarker) Silenced(
	alert model.Fingerprint,
) (activeIDs, pendingIDs []string, version int, silenced bool) {
	return s.marker.Silenced(alert)
}

func (s *StateAwareMarker) Inhibited(alert model.Fingerprint) ([]string, bool) {
	return s.marker.Inhibited(alert)
}

func (s *StateAwareMarker) Muted(routeID, groupKey string) ([]string, bool) {
	return s.marker.Muted(routeID, groupKey)
}

func (s *StateAwareMarker) SetMuted(routeID, groupKey string, timeIntervalNames []string) {
	s.marker.SetMuted(routeID, groupKey, timeIntervalNames)
}

func (s *StateAwareMarker) DeleteByGroupKey(routeID, groupKey string) {
	s.marker.DeleteByGroupKey(routeID, groupKey)
}

func (s *StateAwareMarker) Flush() error {
	return s.appender.Flush()
}
