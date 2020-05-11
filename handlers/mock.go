package handlers

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	"github.com/go-openapi/runtime"
	"github.com/go-openapi/runtime/middleware"
	"github.com/google/uuid"
	"github.com/xan-mortum/apimediaservice/components/imagemanager"
	"github.com/xan-mortum/apimediaservice/gen/models"
	"github.com/xan-mortum/apimediaservice/gen/restapi/operations"
	"github.com/xan-mortum/apimediaservice/interfaces"
	"github.com/xan-mortum/apimediaservice/repositories"
	"path/filepath"
)

type MockHandler struct {
	Logger              interfaces.Logger
	ImageManager        imagemanager.ImageManager
	S3Session           *session.Session
	ImageRepository     *repositories.ImageRepository
	UserImageRepository *repositories.UserImageRepository
}

func NewMockHandler(
	logger interfaces.Logger,
	im imagemanager.ImageManager,
	s3 *session.Session,
	userImageRepository *repositories.UserImageRepository,
	imageRepository *repositories.ImageRepository,
) *MockHandler {
	return &MockHandler{
		Logger: logger,
		ImageManager:        im,
		S3Session:           s3,
		UserImageRepository: userImageRepository,
		ImageRepository:     imageRepository,
	}
}

//метод для получения случайного токена. хотя писать можно любой токен
func (handler *MockHandler) TokenHandler(params operations.TokenParams) middleware.Responder {
	return operations.NewTokenOK().WithPayload(uuid.New().String())
}

//метод для загрузки файлов
func (handler *MockHandler) UploadHandler(params operations.UploadParams) middleware.Responder {
	inputFileData := params.Upfile
	inputToken := params.Token

	fileName := inputFileData.(*runtime.File).Header.Filename
	fileExt := filepath.Ext(fileName)
	//такая структура нужна потому что ReadCloser.Close() выбрасывает ошибку которую тоже нужно обработать
	//универсального решения этой проблемы нет
	defer func() {
		err := inputFileData.Close()
		if err != nil {
			handler.Logger.Warning(err)
		}
	}()

	//проверям расширение картинки что бы не продолжать если файл не подходит
	if !handler.ImageManager.IsExtensionSupported(fileExt) {
		return operations.NewUploadBadRequest().WithPayload(&models.Error{Detail: fileExt + " id not supported"})
	}

	//заливаем на S3
	uploader := s3manager.NewUploader(handler.S3Session)
	upload, err := uploader.Upload(&s3manager.UploadInput{
		Bucket: aws.String(Bucket),
		Key:    aws.String(fileName),
		Body:   params.Upfile.(*runtime.File),
	})
	if err != nil {
		return operations.NewUploadInternalServerError().WithPayload(&models.Error{Detail: err.Error()})
	}

	//сохраняем файл в базу
	err = handler.ImageRepository.Put(repositories.Image{
		Uuid:     fileName,
		FileName: fileName,
		FilePath: upload.Location,
	})
	if err != nil {
		return operations.NewUploadInternalServerError().WithPayload(&models.Error{Detail: err.Error()})
	}

	images := []repositories.UserImage{{
		Uuid:             inputToken,
		OriginalFileName: fileName,
		OriginalFilePath: upload.Location,
	}}

	err = handler.UserImageRepository.Append(images, inputToken)
	if err != nil {
		return operations.NewUploadInternalServerError().WithPayload(&models.Error{Detail: err.Error()})
	}

	return operations.NewUploadOK().WithPayload(fileName)
}