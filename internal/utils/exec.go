package utils

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	log "github.com/sirupsen/logrus"
)

// Command create a command at the project root.
func Command(name string, args ...string) *exec.Cmd {
	cmd := exec.Command(name, args...)

	// By default set the working directory to the project root directory.
	wd, _ := os.Getwd()
	for !strings.HasSuffix(wd, "authelia") {
		wd = filepath.Dir(wd)
	}

	cmd.Dir = wd

	return cmd
}

// CommandWithStdout create a command forwarding stdout and stderr to the OS streams.
func CommandWithStdout(name string, args ...string) *exec.Cmd {
	cmd := Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd
}

// Shell create a shell command.
func Shell(command string) *exec.Cmd {
	return CommandWithStdout("bash", "-c", command)
}

// RunCommandUntilCtrlC run a command until ctrl-c is hit.
func RunCommandUntilCtrlC(cmd *exec.Cmd) {
	mutex := sync.Mutex{}
	cond := sync.NewCond(&mutex)
	signalChannel := make(chan os.Signal)
	signal.Notify(signalChannel, os.Interrupt, syscall.SIGTERM)

	mutex.Lock()

	go func() {
		mutex.Lock()

		f := bufio.NewWriter(os.Stdout)
		defer f.Flush()

		fmt.Println("Hit Ctrl+C to shutdown...")

		err := cmd.Run()

		if err != nil {
			fmt.Println(err)
			cond.Broadcast()
			mutex.Unlock()

			return
		}

		<-signalChannel
		cond.Broadcast()
		mutex.Unlock()
	}()

	cond.Wait()
}

// RunFuncUntilCtrlC run a function until ctrl-c is hit.
func RunFuncUntilCtrlC(fn func() error) error {
	mutex := sync.Mutex{}
	cond := sync.NewCond(&mutex)
	errorChannel := make(chan error)
	signalChannel := make(chan os.Signal)
	signal.Notify(signalChannel, os.Interrupt, syscall.SIGTERM)

	mutex.Lock()

	go func() {
		mutex.Lock()

		f := bufio.NewWriter(os.Stdout)
		defer f.Flush()

		fmt.Println("Hit Ctrl+C to shutdown...")

		err := fn()

		if err != nil {
			errorChannel <- err
			fmt.Println(err)
			cond.Broadcast()
			mutex.Unlock()

			return
		}

		errorChannel <- nil

		<-signalChannel
		cond.Broadcast()
		mutex.Unlock()
	}()

	cond.Wait()

	return <-errorChannel
}

// RunCommandWithTimeout run a command with timeout.
func RunCommandWithTimeout(cmd *exec.Cmd, timeout time.Duration) error {
	// Start a process:
	if err := cmd.Start(); err != nil {
		log.Fatal(err)
	}

	// Wait for the process to finish or kill it after a timeout (whichever happens first):
	done := make(chan error, 1)

	go func() {
		done <- cmd.Wait()
	}()

	select {
	case <-time.After(timeout):
		fmt.Printf("Timeout of %ds reached... Killing process...\n", int64(timeout/time.Second))

		if err := cmd.Process.Kill(); err != nil {
			return err
		}

		return ErrTimeoutReached
	case err := <-done:
		return err
	}
}

// RunFuncWithRetry run a function for n attempts with a sleep of n duration between each attempt.
func RunFuncWithRetry(attempts int, sleep time.Duration, f func() error) (err error) {
	for i := 0; ; i++ {
		err = f()
		if err == nil {
			return
		}

		if i >= (attempts - 1) {
			break
		}

		time.Sleep(sleep)

		log.Printf("Retrying after error: %s", err)
	}

	return fmt.Errorf("Failed after %d attempts, last error: %s", attempts, err)
}