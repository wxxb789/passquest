//go:build windows

package desktop

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

// killChildrenOnExit puts this process in a job object that kills every
// process still in it when the app exits: a Node host the app started (coach,
// model test, Anki push, durable brief) cannot outlive the window. The job
// handle is deliberately never closed; the system closes it at exit.
// Processes that ask to break away (BREAKAWAY_OK) may.
func killChildrenOnExit() error {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return err
	}
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{
		BasicLimitInformation: windows.JOBOBJECT_BASIC_LIMIT_INFORMATION{
			LimitFlags: windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE | windows.JOB_OBJECT_LIMIT_BREAKAWAY_OK,
		},
	}
	if _, err := windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info))); err != nil {
		windows.CloseHandle(job)
		return err
	}
	if err := windows.AssignProcessToJobObject(job, windows.CurrentProcess()); err != nil {
		windows.CloseHandle(job)
		return err
	}
	return nil
}
