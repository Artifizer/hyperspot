package job

import (
	"github.com/hypernetix/hyperspot/libs/fsm"
)

const (
	// JobContextController represents API-triggered transitions (job controller)
	JobContextController fsm.TransitionContext = "controller"
	// ContextExecutor represents internal system transitions (job executor)
	JobContextExecutor fsm.TransitionContext = "executor"
	// JobContextWorker represents job worker internal transitions
	JobContextWorker fsm.TransitionContext = "worker"
)

// contextKey is a custom type for context keys to avoid collisions
type contextKey string

const (
	JobContextKey contextKey = "job_context"
)

// JobFSM holds the finite state machine for job transitions
var JobFSM *fsm.FSM

// initJobFSM initializes the job state machine with all transition rules
func initJobFSM() {
	JobFSM = fsm.NewFSM()

	// Add all job states
	JobFSM.AddStates(
		fsm.State(StatusInit),
		fsm.State(StatusWaiting),
		fsm.State(StatusRunning),
		fsm.State(StatusSuspended),
		fsm.State(StatusLocked),
		fsm.State(StatusSkipped),
		fsm.State(StatusCanceled),
		fsm.State(StatusFailed),
		fsm.State(StatusTimedOut),
		fsm.State(StatusCompleted),
		fsm.State(StatusDeleted),
	)

	// Define Controller context transitions (API-triggered)
	err := JobFSM.AddTransitionRules(JobContextController, fsm.TransitionConfig{
		Allowed: []fsm.TransitionPair{
			{From: []fsm.State{fsm.State(StatusInit)}, To: []fsm.State{
				fsm.State(StatusWaiting),
				fsm.State(StatusSuspended),
				fsm.State(StatusCanceled),
				fsm.State(StatusDeleted),
			}},
			{From: []fsm.State{fsm.State(StatusWaiting)}, To: []fsm.State{
				fsm.State(StatusSuspended),
				fsm.State(StatusCanceled),
				fsm.State(StatusDeleted),
			}},
			{From: []fsm.State{fsm.State(StatusRunning)}, To: []fsm.State{fsm.State(StatusSuspending), fsm.State(StatusCanceling)}},
			{From: []fsm.State{fsm.State(StatusSuspending)}, To: []fsm.State{}}, // No direct controller transitions from suspending
			{From: []fsm.State{fsm.State(StatusSuspended)}, To: []fsm.State{fsm.State(StatusResuming), fsm.State(StatusCanceled), fsm.State(StatusDeleted)}},
			{From: []fsm.State{fsm.State(StatusResuming)}, To: []fsm.State{fsm.State(StatusCanceling)}},
			{From: []fsm.State{fsm.State(StatusLocked)}, To: []fsm.State{fsm.State(StatusSuspending), fsm.State(StatusCanceling)}},
			{From: []fsm.State{
				fsm.State(StatusSkipped),
				fsm.State(StatusCanceled),
				fsm.State(StatusFailed),
				fsm.State(StatusTimedOut),
				fsm.State(StatusCompleted),
			}, To: []fsm.State{fsm.State(StatusDeleted)}},
		},

		Meaningless: []fsm.TransitionPair{
			{From: []fsm.State{fsm.State(StatusRunning)}, To: []fsm.State{fsm.State(StatusResuming)}},
			{From: []fsm.State{
				fsm.State(StatusSuspended),
				fsm.State(StatusSkipped),
				fsm.State(StatusCanceling),
				fsm.State(StatusCanceled),
				fsm.State(StatusFailed),
				fsm.State(StatusTimedOut),
				fsm.State(StatusCompleted),
				fsm.State(StatusDeleted),
			}, To: []fsm.State{fsm.State(StatusSuspending)}},
			{From: []fsm.State{
				fsm.State(StatusSkipped),
				fsm.State(StatusCanceled),
				fsm.State(StatusFailed),
				fsm.State(StatusTimedOut),
				fsm.State(StatusCompleted),
				fsm.State(StatusDeleted),
			}, To: []fsm.State{fsm.State(StatusCanceling)}},
		},

		Retry: []fsm.TransitionPair{
			// Transitions that require retry/waiting
			{From: []fsm.State{fsm.State(StatusInit)}, To: []fsm.State{fsm.State(StatusSuspending), fsm.State(StatusCanceling)}},
			{From: []fsm.State{
				fsm.State(StatusRunning),
				fsm.State(StatusLocked),
				fsm.State(StatusCanceling),
			}, To: []fsm.State{fsm.State(StatusDeleted)}},
			{From: []fsm.State{fsm.State(StatusResuming)}, To: []fsm.State{fsm.State(StatusSuspending), fsm.State(StatusDeleted)}},
			{From: []fsm.State{fsm.State(StatusSuspending)}, To: []fsm.State{fsm.State(StatusResuming), fsm.State(StatusCanceling), fsm.State(StatusDeleted)}},
		},
	})

	if err != nil {
		panic("Failed to add Controller transition rules: " + err.Error())
	}

	// Define Executor context transitions (internal system transitions)
	err = JobFSM.AddTransitionRules(JobContextExecutor, fsm.TransitionConfig{
		Allowed: []fsm.TransitionPair{
			{From: []fsm.State{fsm.State(StatusWaiting)}, To: []fsm.State{
				fsm.State(StatusRunning),
			}},
			{From: []fsm.State{fsm.State(StatusRunning)}, To: []fsm.State{
				fsm.State(StatusCanceling),  // FIXME: must be initiated by Controller only
				fsm.State(StatusSuspending), // FIXME: must be initiated by Controller only
				fsm.State(StatusTimedOut),
				fsm.State(StatusFailed),
				fsm.State(StatusCompleted),
			}},
			{From: []fsm.State{fsm.State(StatusSuspending)}, To: []fsm.State{
				fsm.State(StatusSuspended),
				fsm.State(StatusTimedOut),
				fsm.State(StatusFailed),
				fsm.State(StatusCompleted),
			}},
			{From: []fsm.State{fsm.State(StatusResuming)}, To: []fsm.State{fsm.State(StatusWaiting)}},
			{From: []fsm.State{fsm.State(StatusLocked)}, To: []fsm.State{fsm.State(StatusRunning), fsm.State(StatusFailed)}},
			{From: []fsm.State{fsm.State(StatusCanceling)}, To: []fsm.State{
				fsm.State(StatusCanceled),
				fsm.State(StatusTimedOut),
				fsm.State(StatusFailed),
				fsm.State(StatusCompleted),
			}},
		},
	})

	if err != nil {
		panic("Failed to add Executor transition rules: " + err.Error())
	}

	// Define Worker context transitions (job worker internal transitions)
	err = JobFSM.AddTransitionRules(JobContextWorker, fsm.TransitionConfig{
		Allowed: []fsm.TransitionPair{
			{From: []fsm.State{fsm.State(StatusRunning)}, To: []fsm.State{fsm.State(StatusLocked), fsm.State(StatusSkipped), fsm.State(StatusFailed), fsm.State(StatusCompleted)}},
			{From: []fsm.State{fsm.State(StatusSuspending)}, To: []fsm.State{fsm.State(StatusLocked), fsm.State(StatusSkipped), fsm.State(StatusFailed), fsm.State(StatusCompleted)}},
			{From: []fsm.State{fsm.State(StatusCanceling)}, To: []fsm.State{fsm.State(StatusLocked), fsm.State(StatusSkipped), fsm.State(StatusFailed), fsm.State(StatusCompleted)}},
		},
	})

	if err != nil {
		panic("Failed to add Worker transition rules: " + err.Error())
	}
}

// CheckJobTransition checks if a job status transition is valid
func CheckJobTransition(context fsm.TransitionContext, from, to JobStatus) fsm.TransitionRule {
	if JobFSM == nil {
		initJobFSM()
	}
	return JobFSM.CheckTransition(context, fsm.State(from), fsm.State(to))
}

// CanTransitionJob checks if a job can transition from one status to another
func CanTransitionJob(context fsm.TransitionContext, from, to JobStatus) bool {
	rule := CheckJobTransition(context, from, to)
	return rule.Allowed
}

// RequiresRetryJob checks if a job transition requires retry
func RequiresRetryJob(context fsm.TransitionContext, from, to JobStatus) bool {
	rule := CheckJobTransition(context, from, to)
	return rule.RequiresRetry
}

// IsMeaninglessTransitionJob checks if a job transition is meaningless
func IgnoreJobTransition(context fsm.TransitionContext, from, to JobStatus) bool {
	rule := CheckJobTransition(context, from, to)
	return rule.Type == fsm.TransitionRuleTypeMeaningless
}

// IsMeaninglessTransitionJob checks if a job transition is meaningless (alias for IgnoreJobTransition)
func IsMeaninglessTransitionJob(context fsm.TransitionContext, from, to JobStatus) bool {
	return IgnoreJobTransition(context, from, to)
}
