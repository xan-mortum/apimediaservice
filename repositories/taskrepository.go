package repositories

import (
	"encoding/json"
	"github.com/syndtr/goleveldb/leveldb"
	"sync"
)

const taskKey = "task"

var taskRepositoryInstance *taskRepositoryPrivate

type TaskRepository struct {
	rp *taskRepositoryPrivate
}

func NewTaskRepository(db *leveldb.DB) *TaskRepository {
	if taskRepositoryInstance == nil {
		taskRepositoryInstance = &taskRepositoryPrivate{
			db: db,
		}
	}

	return &TaskRepository{
		rp: taskRepositoryInstance,
	}
}

type taskRepositoryPrivate struct {
	mx sync.Mutex
	db *leveldb.DB
}

func (r *TaskRepository) Get(taskId string) (*Task, error) {
	r.rp.mx.Lock()
	defer r.rp.mx.Unlock()
	has, err := r.rp.db.Has([]byte(taskKey+":"+taskId), nil)
	if err != nil {
		return nil, err
	}
	if !has {
		return nil, nil
	}
	data, err := r.rp.db.Get([]byte(taskKey+":"+taskId), nil)
	var result Task
	err = json.Unmarshal(data, &result)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func (r *TaskRepository) Put(task Task, taskId string) error {
	r.rp.mx.Lock()
	defer r.rp.mx.Unlock()

	data, err := json.Marshal(task)
	if err != nil {
		return err
	}
	err = r.rp.db.Put([]byte(taskKey+":"+taskId), data, nil)
	if err != nil {
		return err
	}

	return nil
}

const StatusDone = "done"
const StatusInProgress = "in_progress"
const StatusError = "error"

type Task struct {
	Status          string
	FileName        string `json:"fileName"`
	FilePath        string `json:"filePath"`
	ResizedFileName string `json:"resizedFileName"`
	ResizedFilePath string `json:"resizedFilePath"`
	Error           string `json:"error"`
}
