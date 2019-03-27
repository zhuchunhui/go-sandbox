package main

import (
	"log"
	"os"
	"runtime"
	"time"

	libseccomp "github.com/seccomp/libseccomp-golang"
	unix "golang.org/x/sys/unix"
)

var (
	defaultAllows = []string{
		"read",
		"write",
		"readv",
		"writev",
		"open",
		"unlink",
		"close",
		"readlink",
		"openat",
		"unlinkat",
		"readlinkat",
		"stat",
		"fstat",
		"lstat",
		"lseek",
		"access",
		"dup",
		"dup2",
		"dup3",
		"ioctl",
		"fcntl",

		"mmap",
		"mprotect",
		"munmap",
		"brk",
		"mremap",
		"msync",
		"mincore",
		"madvise",

		"rt_sigaction",
		"rt_sigprocmask",
		"rt_sigreturn",
		"rt_sigpending",
		"sigaltstack",

		"getcwd",

		"exit",
		"exit_group",

		"arch_prctl",

		"gettimeofday",
		"getrlimit",
		"getrusage",
		"times",
		"time",
		"clock_gettime",

		"restart_syscall",
	}
	defaultTraces = []string{
		"execve",
	}
)

func buildFilter(allows, traces []string) (*libseccomp.ScmpFilter, error) {
	// make filter
	//filter, err := libseccomp.NewFilter(libseccomp.ActErrno.SetReturnCode(int16(syscall.EPERM)))
	filter, err := libseccomp.NewFilter(libseccomp.ActTrace.SetReturnCode(100))
	if err != nil {
		return nil, err
	}

	for _, s := range allows {
		//log.Println("[+] allow syscall: ", s)
		syscallId, err := libseccomp.GetSyscallFromName(s)
		if err != nil {
			return nil, err
		}
		filter.AddRule(syscallId, libseccomp.ActAllow)
	}

	for _, s := range traces {
		//log.Println("[+] trace syscall: ", s)
		syscallId, err := libseccomp.GetSyscallFromName(s)
		if err != nil {
			return nil, err
		}
		//filter.AddRule(syscallId, libseccomp.ActAllow)
		filter.AddRule(syscallId, libseccomp.ActTrace.SetReturnCode(10))
	}
	return filter, nil
}

func main() {
	// Ptrace require running at the same thread
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	filter, err := buildFilter(defaultAllows, defaultTraces)
	if err != nil {
		log.Fatal("Failed to create filter: ", err)
	}
	// run in restricted mode
	pid, err := ForkAndLoadSeccomp(os.Args[1:], filter)
	if err != nil {
		log.Fatal("Failed to fork: ", err)
	}
	log.Println("After fork")

	// Set real time limit
	timer := time.AfterFunc(time.Duration(1e9), func() {
		log.Println("Before kill")
		unix.Kill(pid, unix.SIGKILL)
		log.Println("After kill")
	})
	defer timer.Stop()

	// Set trace seccomp
	unix.PtraceSetOptions(pid, unix.PTRACE_O_TRACESECCOMP)
	log.Println("Strat trace pid: ", pid)

	// trace unixs
	for {
		var wstatus unix.WaitStatus
		var rusage unix.Rusage

		_, err := unix.Wait4(pid, &wstatus, unix.WALL, &rusage)
		if err != nil {
			log.Fatalln("Wait4 fatal: ", err)
		}
		if wstatus.Exited() {
			log.Println("Exited", wstatus.ExitStatus())
			break
		}
		if wstatus.Signaled() {
			log.Println("Signal", wstatus.Signal())
		}
		if wstatus.Stopped() {
			log.Println("Stopped")
			if wstatus.TrapCause() == unix.PTRACE_EVENT_SECCOMP {
				log.Println("Seccomp Traced")
				msg, err := unix.PtraceGetEventMsg(pid)
				if err != nil {
					log.Fatalln(err)
				}
				log.Println("Ptrace Event: ", msg)
			} else {
				log.Println("Stop Cause: ", wstatus.TrapCause())
			}
		}

		log.Println("Ptrace continue")
		unix.PtraceCont(pid, 0)
	}
}