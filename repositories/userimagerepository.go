package repositories

import (
	"encoding/json"
	"github.com/syndtr/goleveldb/leveldb"
	"sync"
)

const userImagesKey = "userImages"

var userImageRepositoryInstance *userImageRepositoryPrivate

type UserImageRepository struct {
	rp *userImageRepositoryPrivate
}

func NewUserImageRepository(db *leveldb.DB) *UserImageRepository {
	if userImageRepositoryInstance == nil {
		userImageRepositoryInstance = &userImageRepositoryPrivate{
			db: db,
		}
	}

	return &UserImageRepository{
		rp: userImageRepositoryInstance,
	}
}

type userImageRepositoryPrivate struct {
	mx sync.Mutex
	db *leveldb.DB
}

func (r *UserImageRepository) Get(userToken string) ([]UserImage, error) {
	r.rp.mx.Lock()
	defer r.rp.mx.Unlock()
	has, err := r.rp.db.Has([]byte(userImagesKey+":"+userToken), nil)
	if err != nil {
		return []UserImage{}, err
	}
	if !has {
		return []UserImage{}, nil
	}
	data, err := r.rp.db.Get([]byte(userImagesKey+":"+userToken), nil)
	var userImages []UserImage
	err = json.Unmarshal(data, &userImages)
	if err != nil {
		return []UserImage{}, err
	}
	return userImages, nil
}

func (r *UserImageRepository) Put(userImages []UserImage, userToken string) error {
	r.rp.mx.Lock()
	defer r.rp.mx.Unlock()

	data, err := json.Marshal(userImages)
	if err != nil {
		return err
	}
	err = r.rp.db.Put([]byte(userImagesKey+":"+userToken), data, nil)
	if err != nil {
		return err
	}
	return nil
}

func (r *UserImageRepository) Append(userImages []UserImage, userToken string) error {
	r.rp.mx.Lock()
	defer r.rp.mx.Unlock()
	var images []UserImage
	has, err := r.rp.db.Has([]byte(userImagesKey+":"+userToken), nil)
	if err != nil {
		return err
	}
	if has {
		oldImages, err := r.rp.db.Get([]byte(userImagesKey+":"+userToken), nil)
		err = json.Unmarshal(oldImages, &images)
		if err != nil {
			return err
		}
	}

	userImages = append(userImages, images...)

	allImagesJson, err := json.Marshal(userImages)
	err = r.rp.db.Put([]byte(userImagesKey+":"+userToken), allImagesJson, nil)
	if err != nil {
		return err
	}
	return nil
}

type UserImage struct {
	Uuid             string             `json:"uuid"`
	OriginalFileName string             `json:"originalFileName"`
	OriginalFilePath string             `json:"originalFilePath"`
	Resized          []ImageResizeInfo `json:"resized"`
}
