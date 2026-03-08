package main

import (
	"encoding/json"
	"strings"
	"time"

	nomadApi "github.com/hashicorp/nomad/api"
)

// BACKOFF_DELAY rate-limits streaming logs of tasks that error (BACKOFF_DELAY << error-count) seconds
var BACKOFF_DELAY = 8

// NomadLog annotates task log data with metadata from Nomad about the task and allocation.
type NomadLog struct {
	AllocId  string            `json:"alloc_id"`
	JobName  string            `json:"job_name"`
	NodeName string            `json:"node_name"`
	TaskMeta map[string]string `json:"-"`
	TaskName string            `json:"task_name"`
	// these all set at log time
	Timestamp string                 `json:"timestamp"`
	Message   string                 `json:"message"`
}

func (n *NomadLog) ToJSON() (string, error) {
	result, err := json.Marshal(n)
	if err != nil {
		return "", err
	}
	return string(result), nil
}

// FollowedTask a container for a followed task log process
type FollowedTask struct {
	Alloc       *nomadApi.Allocation
	TaskGroup   string
	Task        *nomadApi.Task
	Nomad       NomadConfig
	Quit        chan struct{}
	OutputChan  chan string
	log         Logger
	logTemplate NomadLog
	errState    StreamState
	outState    StreamState
}

// NewFollowedTask creates a new followed task
func NewFollowedTask(alloc *nomadApi.Allocation, taskGroup string, task *nomadApi.Task, nomad NomadConfig, quit chan struct{}, output chan string, logger Logger) *FollowedTask {
	logTemplate := createLogTemplate(alloc, task)
	return &FollowedTask{
		Alloc:       alloc,
		TaskGroup:   taskGroup,
		Task:        task,
		Nomad:       nomad,
		Quit:        quit,
		OutputChan:  output,
		log:         logger,
		logTemplate: logTemplate,
	}
}

type StreamState struct {
	ConsecErrors uint
	FileOffsets  map[string]int64
	// internal use only
	quit       chan struct{}
	allocFS    *nomadApi.AllocFS
}

func (s *StreamState) SetOffsets(offsets map[string]int64) {
	if offsets != nil {
		s.FileOffsets = offsets
	} else {
		s.FileOffsets = make(map[string]int64)
	}
}

// GetOffset returns the current total log stream offset.
func (s *StreamState) GetOffset() int64 {
	var offset int64
	offset = 0
	// sum the byte offsets for all of the file streams
	for _, o := range s.FileOffsets {
		offset += o
	}
	return offset
}

func (s *StreamState) Start(alloc *nomadApi.Allocation, task, logType string) (<-chan *nomadApi.StreamFrame, <-chan error) {
	return s.allocFS.Logs(alloc, true, task, logType, "start", s.GetOffset(), s.quit, &nomadApi.QueryOptions{})
}

func NewStreamState(allocFS *nomadApi.AllocFS, quit chan struct{}) StreamState {
	s := StreamState{}
	s.quit = quit
	s.allocFS = allocFS
	s.FileOffsets = make(map[string]int64)
	return s
}

// Start starts following a task for an allocation
func (ft *FollowedTask) Start(save *SavedTask) {
	logContext := "FollowedTask.Start"
	fs := ft.Nomad.Client().AllocFS()
	ft.outState = NewStreamState(fs, ft.Quit)
	ft.errState = NewStreamState(fs, ft.Quit)
	if save != nil {
		ft.log.Debugf(logContext, "Restoring log offsets for %s", ft.Task.Name)
		ft.outState.SetOffsets(save.StdOutOffsets)
		ft.errState.SetOffsets(save.StdErrOffsets)
	}
	stdOutCh, stdOutErr := ft.outState.Start(ft.Alloc, ft.Task.Name, "stdout")
	stdErrCh, stdErrErr := ft.errState.Start(ft.Alloc, ft.Task.Name, "stderr")

	go func() {
		for {
			// Backoff on consecutive errors to avoid crash looping
			if ft.outState.ConsecErrors > 0 || ft.errState.ConsecErrors > 0 {
				delay := BACKOFF_DELAY << (ft.outState.ConsecErrors + ft.errState.ConsecErrors)
				ft.log.Debugf(
					logContext,
					"Inserting delay of %ds for alloc: %s task: %s",
					delay,
					ft.Alloc.ID,
					ft.Task.Name,
				)
				time.Sleep(time.Duration(delay) * time.Second)
			}

			select {
			case _, ok := <-ft.Quit:
				if !ok {
					return
				}
			case stdErrFrame, stdErrOk := <-stdErrCh:
				if stdErrOk {
					ft.processFrame(stdErrFrame)
					ft.errState.FileOffsets[stdErrFrame.File] += int64(len(stdErrFrame.Data))
					ft.errState.ConsecErrors = 0
					ft.outState.ConsecErrors = 0
				} else {
					stdErrCh, stdErrErr = ft.errState.Start(ft.Alloc, ft.Task.Name, "stderr")
					ft.errState.ConsecErrors += 1
				}

			case stdOutFrame, stdOutOk := <-stdOutCh:
				if stdOutOk {
					ft.processFrame(stdOutFrame)
					ft.outState.FileOffsets[stdOutFrame.File] += int64(len(stdOutFrame.Data))
					ft.outState.ConsecErrors = 0
					ft.errState.ConsecErrors = 0
				} else {
					stdOutCh, stdOutErr = ft.outState.Start(ft.Alloc, ft.Task.Name, "stdout")
					ft.outState.ConsecErrors += 1
				}

			case errErr := <-stdErrErr:
				ft.log.Debugf(
					logContext,
					"Error following stderr alloc: %s task: %s error: %s",
					ft.Alloc.ID,
					ft.Task.Name,
					errErr,
				)
				// TODO handle 403 case separately from 404 case
				// Handle task starting while client has invalid token
				ft.Nomad.RenewToken()
				stdErrCh, stdErrErr = ft.errState.Start(ft.Alloc, ft.Task.Name, "stderr")
				ft.errState.ConsecErrors += 1

			case outErr := <-stdOutErr:
				ft.log.Debugf(
					logContext,
					"Error following stdout alloc: %s task: %s error: %s",
					ft.Alloc.ID,
					ft.Task.Name,
					outErr,
				)
				// TODO handle 403 case separately from 404 case
				// Handle task starting while client has invalid token
				ft.Nomad.RenewToken()
				stdOutCh, stdOutErr = ft.outState.Start(ft.Alloc, ft.Task.Name, "stdout")
				ft.outState.ConsecErrors += 1
			}
		}
	}()
}

func createLogTemplate(alloc *nomadApi.Allocation, task *nomadApi.Task) NomadLog {
	tmpl := NomadLog{}

	tmpl.AllocId = alloc.ID
	tmpl.JobName = *alloc.Job.Name
	tmpl.NodeName = alloc.NodeName
	for _, tg := range alloc.Job.TaskGroups {
		for _, t := range tg.Tasks {
			if t.Name == task.Name {
				tmpl.TaskMeta = t.Meta
			}
		}
	}
	tmpl.TaskName = task.Name
	return tmpl
}

// processFrame processes a stream frame and sends each log line as JSON to the output channel
func (ft *FollowedTask) processFrame(frame *nomadApi.StreamFrame) {
	logContext := "FollowedTask.processFrame"
	messages := strings.Split(string(frame.Data[:]), "\n")
	for _, message := range messages {
		if message == "" || message == "\n" {
			continue
		}
		
		// Create log entry with current timestamp
		log := ft.logTemplate
		log.Timestamp = time.Now().UTC().Format(time.RFC3339)
		log.Message = message
		
		jsonStr, err := log.ToJSON()
		if err != nil {
			ft.log.Errorf(logContext, "Error marshaling log: %v", err)
			continue
		}
		
		ft.OutputChan <- jsonStr
	}
}
