package repositories

import (
	"encoding/json"
	"github.com/syndtr/goleveldb/leveldb"
	"sync"
)

const imagesKey = "images"

var imageRepositoryInstance *imageRepositoryPrivate

type ImageRepository struct {
	rp *imageRepositoryPrivate
}

func NewImageRepository(db *leveldb.DB) *ImageRepository {
	if imageRepositoryInstance == nil {
		imageRepositoryInstance = &imageRepositoryPrivate{
			db: db,
		}
	}

	return &ImageRepository{
		rp: imageRepositoryInstance,
	}
}

type imageRepositoryPrivate struct {
	mx sync.Mutex
	db *leveldb.DB
}

func (r *ImageRepository) Get(image string) (Image, error) {
	r.rp.mx.Lock()
	defer r.rp.mx.Unlock()
	has, err := r.rp.db.Has([]byte(imagesKey + ":" + image), nil)
	if err != nil {
		return Image{}, err
	}
	if !has {
		return Image{}, nil
	}
	data, err := r.rp.db.Get([]byte(imagesKey + ":" + image), nil)
	var result Image
	err = json.Unmarshal(data, &result)
	if err != nil {
		return Image{}, err
	}
	return result, nil
}

func (r *ImageRepository) Put(image Image) error {
	r.rp.mx.Lock()
	defer r.rp.mx.Unlock()

	data, err := json.Marshal(image)
	if err != nil {
		return err
	}
	err = r.rp.db.Put([]byte(imagesKey + ":" + image.Uuid), data, nil)
	if err != nil {
		return err
	}

	return nil
}

type Image struct {
	Uuid     string `json:"uuid"`
	FileName string `json:"fileName"`
	FilePath string `json:"filePath"`
}
