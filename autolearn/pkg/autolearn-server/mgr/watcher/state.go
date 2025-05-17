package watcher

import (
	"fmt"
	"strings"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"

	"go.megvii-inc.com/brain/brainpp/projects/aiservice/autolearn/pkg/autolearn-server/mgr/dao"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/autolearn/pkg/autolearn-server/types"
)

var lingeringTable *LingeringTable
var lock *LockMap

type LingeringTable struct {
	table map[types.AutoLearnState]time.Duration
}

func NewLingeringTable() *LingeringTable {
	if lingeringTable != nil {
		return lingeringTable
	}

	mt := &LingeringTable{
		table: map[types.AutoLearnState]time.Duration{},
	}

	for state, t := range types.LingeringTime {
		mt.table[state] = time.Duration(t*60) * time.Second
	}

	lingeringTable = mt

	return mt
}

func (mt *LingeringTable) HasBreakDown(currentState types.AutoLearnState, startedAt int64) bool {
	elapse := time.Now().Unix() - startedAt
	if v, ok := mt.table[currentState]; ok {
		return elapse > int64(v.Seconds())
	}
	return false
}

type LockMap struct {
	lock sync.Map
}

func NewLockMap() *LockMap {
	if lock != nil {
		return lock
	}

	lk := &LockMap{
		lock: sync.Map{},
	}

	lock = lk

	return lk
}

func (lm *LockMap) GetLock(id string) *sync.Mutex {
	if lock, ok := lm.lock.Load(id); ok {
		return lock.(*sync.Mutex)
	}

	return nil
}

func (lm *LockMap) SetLock(id string) {
	lm.lock.Store(id, &sync.Mutex{})
}

func (lm *LockMap) RemoveLock(id string) {
	lm.lock.Delete(id)
}

type Action func(*dao.DAO, string, types.StateEventCode, ...string) error

type StateMachine struct {
	Lock     *LockMap
	EventMap map[types.StateEventType]map[types.EventAction]Action
}

func NewStateMachine(lock *LockMap) *StateMachine {
	stateAction := func(s1 types.AutoLearnState, s2 types.AutoLearnState) Action {
		return func(d *dao.DAO, rvID string, code types.StateEventCode, msg ...string) error {
			var reason string
			if code == types.StateEventCodeCustom && len(msg) > 0 {
				reason = strings.Join(msg, "\n")
			} else {
				reason = types.EventCodeMessage[code]
			}

			if s1 == -1 && s2 == types.AutoLearnStateStopped {
				return dao.UpdateRevisionState(d, rvID, s2, reason)
			}

			rv, err := dao.GetBriefRevisionByID(d, rvID)
			if err != nil {
				log.WithError(err).Error("find autolearn revision failed")
				return err
			}

			if rv.AutoLearnState == types.AutoLearnStateStopped {
				return fmt.Errorf("revision is already in final state")
			}

			if rv.AutoLearnState == s1 {
				return dao.UpdateRevisionState(d, rvID, s2, reason)
			}

			return nil
		}
	}

	eventMap := map[types.StateEventType]map[types.EventAction]Action{}
	for eventType, actions := range types.EventMap {
		for action, states := range actions {
			if eventMap[eventType] == nil {
				eventMap[eventType] = map[types.EventAction]Action{}
			}
			eventMap[eventType][action] = stateAction(states[0], states[1])
		}
	}

	return &StateMachine{
		Lock:     lock,
		EventMap: eventMap,
	}
}

func (st *StateMachine) Action(d *dao.DAO, event *types.StateEvent, rvID string, code types.StateEventCode, msg ...string) error {
	action, ok := st.EventMap[event.T]
	if !ok {
		return fmt.Errorf("this event is not supported")
	}

	lock := st.Lock.GetLock(rvID)
	if lock == nil {
		return fmt.Errorf("can not change rv: %s state when having no lock", rvID)
	}

	lock.Lock()
	err := action[event.A](d, rvID, code, msg...)
	lock.Unlock()

	if ((event.T == types.StateEventTypeLearn && event.A == types.EventActionSuccess) ||
		code != types.StateEventCodeNothing) && err == nil {
		st.Lock.RemoveLock(rvID)
	}

	return err
}

func (st *StateMachine) SetLock(rvID string) {
	st.Lock.SetLock(rvID)
}
