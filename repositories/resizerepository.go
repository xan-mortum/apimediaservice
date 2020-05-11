package repositories

import (
	"encoding/json"
	"github.com/syndtr/goleveldb/leveldb"
	"sync"
)

const resizeKey = "resize"

var resizeRepositoryInstance *resizeRepositoryPrivate

type ResizeRepository struct {
	rp *resizeRepositoryPrivate
}

func NewResizeRepository(db *leveldb.DB) *ResizeRepository {
	if resizeRepositoryInstance == nil {
		resizeRepositoryInstance = &resizeRepositoryPrivate{
			db: db,
		}
	}

	return &ResizeRepository{
		rp: resizeRepositoryInstance,
	}
}

type resizeRepositoryPrivate struct {
	mx sync.Mutex
	db *leveldb.DB
}

func (r *ResizeRepository) Get(image string) ([]ImageResizeInfo, error) {
	r.rp.mx.Lock()
	defer r.rp.mx.Unlock()
	has, err := r.rp.db.Has([]byte(resizeKey + ":" + image), nil)
	if err != nil {
		return []ImageResizeInfo{}, err
	}
	if !has {
		return []ImageResizeInfo{}, nil
	}
	data, err := r.rp.db.Get([]byte(resizeKey + ":" + image), nil)
	var result []ImageResizeInfo
	err = json.Unmarshal(data, &result)
	if err != nil {
		return []ImageResizeInfo{}, err
	}
	return result, nil
}

func (r *ResizeRepository) Put(resize ImageResizeInfo, image string) error {
	r.rp.mx.Lock()
	defer r.rp.mx.Unlock()

	data, err := json.Marshal(resize)
	if err != nil {
		return err
	}
	err = r.rp.db.Put([]byte(resizeKey + ":" + image), data, nil)
	if err != nil {
		return err
	}
	return nil
}

func (r *ResizeRepository) Append(resize []ImageResizeInfo, image string) error {
	r.rp.mx.Lock()
	defer r.rp.mx.Unlock()
	var imageResizeInfos []ImageResizeInfo
	has, err := r.rp.db.Has([]byte(resizeKey+":"+image), nil)
	if err != nil {
		return err
	}
	if has {
		oldResize, err := r.rp.db.Get([]byte(resizeKey+":"+image), nil)
		err = json.Unmarshal(oldResize, &imageResizeInfos)
		if err != nil {
			return err
		}
	}

	resize = append(resize, imageResizeInfos...)

	allResizeJson, err := json.Marshal(resize)
	err = r.rp.db.Put([]byte(resizeKey+":"+image), allResizeJson, nil)
	if err != nil {
		return err
	}
	return nil
}

type ImageResizeInfo struct {
	ResizedFileName string `json:"resizedFileName"`
	ResizedFilePath string `json:"resizedFilePath"`
	ResizeParam     int64  `json:"resizeParam"`
}