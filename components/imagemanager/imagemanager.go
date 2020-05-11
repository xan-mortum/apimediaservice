package imagemanager

import (
	"errors"
	"github.com/nfnt/resize"
	"image"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
)

const ThumbPrefix = "thumb"
var supportedExtension = map[string]string{".jpg": "jpg", ".jpeg": "jpeg", ".png": "png", ".gif": "gif"}

//структура которая занимаеться манипуляциями с файлами
//сохраняет файлы во временную папку путь к кторой указываеться в конфиге
//находиться в папке components потому что я решил что эта папка будет аналогом папки vendor но только для своих пакетов
//можно было бы использовать встроенное решение для создания временных файлов но это решение платформозависимо
type ImageManager struct {
	Config Config
	TmpFiles []*File
}

func NewImageManager(config Config) ImageManager {
	im := &ImageManager{Config:config}
	//метод который вызываеться при выгрузке обьекта из памяти и чистит временную папку
	runtime.SetFinalizer(im, func(im *ImageManager) {
		im.Clear()
	})
	return *im
}

//для ручной очистки папки
func (im *ImageManager) Clear() {
	for _, file := range im.TmpFiles {
		_ = os.Remove(file.Path)
	}
}

func (im *ImageManager) IsExtensionSupported(extension string) bool {
	_, ok := supportedExtension[extension]
	return ok
}

func (im *ImageManager) SaveFile(fileName string, bytes []byte) (*File, error) {
	file, err := im.CreateEmptyFile(fileName)
	if err != nil {
		return nil, err
	}
	_, err = file.Write(bytes)
	if err != nil {
		//через defer закрывать файлы не получиться, так что лучше так
		_ = file.Close()
		return nil, err
	}

	err = file.Close()
	if err != nil {
		return nil, err
	}

	fileStruct := &File{
		Name:      fileName,
		Path:      im.Config.TmpDir + fileName,
		Extension: filepath.Ext(fileName),
	}
	im.TmpFiles = append(im.TmpFiles, fileStruct)

	return fileStruct, nil
}

func (im *ImageManager) CreateFile(inputFile io.Reader, fileName string) (*File, error) {
	filePath := im.Config.TmpDir + fileName
	file, err := os.Create(filePath)
	if err != nil {
		return nil, err
	}

	_, err = io.Copy(file, inputFile)
	if err != nil {
		_ = file.Close()
		return nil, err
	}

	err = file.Close()
	if err != nil {
		return nil, err
	}

	fileStruct := &File{
		Name:      fileName,
		Path:      filePath,
		Extension: filepath.Ext(filePath),
	}
	im.TmpFiles = append(im.TmpFiles, fileStruct)

	return fileStruct, nil
}

func (im *ImageManager) CreateEmptyFile(fileName string) (*os.File, error) {
	filePath := im.Config.TmpDir + fileName
	file, err := os.Create(filePath)
	if err != nil {
		return nil, err
	}

	return file, nil
}

func (im *ImageManager) ResizeFile(file *File, width uint) (*File, error) {
	fileToDecode, err := os.Open(file.Path)
	if err != nil {
		return nil, err
	}

	decodedImage, _, err := image.Decode(fileToDecode)
	if err != nil {
		return nil, err
	}

	err = fileToDecode.Close()
	if err != nil {
		return nil, err
	}

	thumbImage := resize.Resize(width, 0, decodedImage, resize.Lanczos3)

	thumbFileName := ThumbPrefix + strconv.Itoa(int(width)) + "." + file.Name
	thumbFilePath := im.Config.TmpDir + thumbFileName
	thumbFile, err := os.Create(thumbFilePath)
	if err != nil {
		return nil, err
	}

	fileExt := filepath.Ext(thumbFilePath)
	switch fileExt {
	case ".jpg", ".jpeg":
		err = jpeg.Encode(thumbFile, thumbImage, nil)
	case ".png":
		err = png.Encode(thumbFile, thumbImage)
	case ".gif":
		err = gif.Encode(thumbFile, thumbImage, nil)
	default:
		return nil, errors.New(fileExt + " is not supported")
	}
	if err != nil {
		_ = thumbFile.Close()
		return nil, err
	}

	err = thumbFile.Close()
	if err != nil {
		return nil, err
	}

	thumbFileStruct := &File{
		Name:      thumbFileName,
		Path:      thumbFilePath,
		Extension: fileExt,
	}
	im.TmpFiles = append(im.TmpFiles, thumbFileStruct)

	return thumbFileStruct, nil
}

func (im *ImageManager) GetFileResource(file *File) (*os.File, error) {
	return os.Open(file.Path)
}
