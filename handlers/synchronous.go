package handlers

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
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

type SynchronousHandler struct {
	Logger              interfaces.Logger
	ImageManager        imagemanager.ImageManager
	S3Session           *session.Session
	UserImageRepository *repositories.UserImageRepository
	ImageRepository     *repositories.ImageRepository
	ResizeRepository    *repositories.ResizeRepository
	bucket           string
}

func NewSynchronousHandler(
	logger interfaces.Logger,
	im imagemanager.ImageManager,
	s3 *session.Session,
	userImageRepository *repositories.UserImageRepository,
	imageRepository *repositories.ImageRepository,
	resizeRepository *repositories.ResizeRepository,
	bucket           string,
) *SynchronousHandler {
	return &SynchronousHandler{
		Logger:              logger,
		//структура которая никапсулирует работу с временными файлами в том числе с ресайзнутыми
		ImageManager:        im,
		S3Session:           s3,
		UserImageRepository: userImageRepository,
		ImageRepository:     imageRepository,
		ResizeRepository:    resizeRepository,
		bucket: bucket,
	}
}

//получаем картику с параметрами, резайзим и отправляем ссылки на оа файла
func (handler *SynchronousHandler) ResizeHandler(params operations.ResizeParams) middleware.Responder {
	//ролучаем входящие параметры и файл
	inputFileData := params.Upfile
	inputResize := params.Resize
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
		return operations.NewResizeBadRequest().WithPayload(&models.Error{Detail: fileExt + " id not supported",})
	}

	//тут создаеться временный файл в файловой системе и возвращаеться структура с его данными
	file, err := handler.ImageManager.CreateFile(params.Upfile, fileName)
	if err != nil {
		return operations.NewResizeInternalServerError().WithPayload(&models.Error{Detail: err.Error()})
	}

	//тут создаеться временная картика с измененным размером. возвращаеться структура с данными
	thumbFile, err := handler.ImageManager.ResizeFile(file, uint(inputResize))
	if err != nil {
		return operations.NewResizeInternalServerError().WithPayload(&models.Error{Detail: err.Error()})
	}

	//получаем *os.File
	//на этом уровне мы не должны знать как храняться файлы и что с ними происходит
	//но сам файл получить нужно для дальшейшей работы
	fileToUpload, err := handler.ImageManager.GetFileResource(file)
	defer func() {
		err := fileToUpload.Close()
		if err != nil {
			handler.Logger.Warning(err)
		}
	}()
	if err != nil {
		return operations.NewResizeInternalServerError().WithPayload(&models.Error{Detail: err.Error()})
	}

	//заливаем на S3
	uploader := s3manager.NewUploader(handler.S3Session)
	upload, err := uploader.Upload(&s3manager.UploadInput{
		Bucket: aws.String(handler.bucket),
		Key:    aws.String(file.Name),
		Body:   fileToUpload,
	})
	if err != nil {
		return operations.NewResizeInternalServerError().WithPayload(&models.Error{Detail: err.Error()})
	}

	//получаем теперь измененныую картинку
	thumbToUpload, err := handler.ImageManager.GetFileResource(thumbFile)
	defer func() {
		err := thumbToUpload.Close()
		if err != nil {
			handler.Logger.Warning(err)
		}
	}()
	if err != nil {
		return operations.NewResizeInternalServerError().WithPayload(&models.Error{Detail: err.Error()})
	}

	//заливаем на S3
	thumbUpload, err := uploader.Upload(&s3manager.UploadInput{
		Bucket: aws.String(handler.bucket),
		Key:    aws.String(thumbFile.Name),
		Body:   thumbToUpload,
	})
	if err != nil {
		return operations.NewResizeInternalServerError().WithPayload(&models.Error{Detail: err.Error()})
	}

	//этот метод чистит временную папку. его вызывать не обязательно, есть метод автоматического удаления
	handler.ImageManager.Clear()

	//сохраняем все в базу
	imageUuid := uuid.New().String()
	err = handler.ImageRepository.Put(repositories.Image{
		Uuid:     imageUuid,
		FileName: file.Name,
		FilePath: upload.Location,
	})
	if err != nil {
		return operations.NewResizeInternalServerError().WithPayload(&models.Error{Detail: err.Error()})
	}

	images := []repositories.UserImage{{
		Uuid:             imageUuid,
		OriginalFileName: file.Name,
		OriginalFilePath: upload.Location,
	}}

	err = handler.UserImageRepository.Append(images, inputToken)
	if err != nil {
		return operations.NewResizeInternalServerError().WithPayload(&models.Error{Detail: err.Error()})
	}

	err = handler.ResizeRepository.Append([]repositories.ImageResizeInfo{{
		ResizedFileName: thumbFile.Name,
		ResizedFilePath: thumbUpload.Location,
		ResizeParam:     inputResize,
	}}, imageUuid)
	if err != nil {
		return operations.NewResizeInternalServerError().WithPayload(&models.Error{Detail: err.Error()})
	}

	return operations.NewResizeOK().WithPayload(&models.Resize{
		Original: upload.Location,
		Resized:  thumbUpload.Location,
	})
}

//возвращаем список всех файлов пользователя
func (handler *SynchronousHandler) FilesHandler(params operations.FilesParams) middleware.Responder {
	//по этому токену определяем пользователя. писать можно любую стоку при ресайзе, и потом ее же присылать сюда
	inputToken := params.Token

	//получаем из базы информацию о картинках пользователя
	files, err := handler.UserImageRepository.Get(inputToken)
	if err != nil {
		return operations.NewFilesBadRequest().WithPayload(&models.Error{Detail: err.Error()})
	}

	//получаем резайзы картинок
	var result []repositories.UserImage
	for _, file := range files {
		resizeInfo, err := handler.ResizeRepository.Get(file.Uuid)
		if err != nil {
			return operations.NewFilesBadRequest().WithPayload(&models.Error{Detail: err.Error()})
		}
		file.Resized = append(file.Resized, resizeInfo...)
		result = append(result, file)
	}

	return operations.NewFilesOK().WithPayload(result)
}

//ресайзим уже загруженную картинку
//uuid картинки можно получить вызовом /files
func (handler *SynchronousHandler) ResizeExistsHandler(params operations.ResizeExistsParams) middleware.Responder {
	//inputToken := params.Token
	inputFile := params.File
	inputResize := params.Resize

	//получаем картинку из базы
	fileInfo, err := handler.ImageRepository.Get(inputFile)
	if err != nil {
		return operations.NewResizeExistsBadRequest().WithPayload(&models.Error{Detail: err.Error()})
	}

	//скачиваем картинку с S3
	buf := aws.NewWriteAtBuffer([]byte{})
	downloader := s3manager.NewDownloader(handler.S3Session)
	_, err = downloader.Download(buf, &s3.GetObjectInput{
		Bucket: aws.String(handler.bucket),
		Key:    aws.String(fileInfo.FileName),
	})
	if err != nil {
		return operations.NewResizeExistsInternalServerError().WithPayload(&models.Error{Detail: err.Error()})
	}

	//сохраняем файл во временную папку
	downloadedFile, err := handler.ImageManager.SaveFile(fileInfo.FileName, buf.Bytes())
	if err != nil {
		return operations.NewResizeExistsInternalServerError().WithPayload(&models.Error{Detail: err.Error()})
	}

	//ресайзим картинку
	thumbFile, err := handler.ImageManager.ResizeFile(downloadedFile, uint(inputResize))
	if err != nil {
		return operations.NewResizeInternalServerError().WithPayload(&models.Error{Detail: err.Error()})
	}

	//получаем ссылку на файл
	thumbToUpload, err := handler.ImageManager.GetFileResource(thumbFile)
	defer func() {
		err := thumbToUpload.Close()
		if err != nil {
			handler.Logger.Warning(err)
		}
	}()
	if err != nil {
		return operations.NewResizeInternalServerError().WithPayload(&models.Error{Detail: err.Error()})
	}

	//загруаем на S3
	uploader := s3manager.NewUploader(handler.S3Session)
	thumbUpload, err := uploader.Upload(&s3manager.UploadInput{
		Bucket: aws.String(handler.bucket),
		Key:    aws.String(thumbFile.Name),
		Body:   thumbToUpload,
	})
	if err != nil {
		return operations.NewResizeInternalServerError().WithPayload(&models.Error{Detail: err.Error()})
	}

	//чистим папку
	handler.ImageManager.Clear()

	//сохраняем в базу
	err = handler.ResizeRepository.Append([]repositories.ImageResizeInfo{{
		ResizedFileName: thumbFile.Name,
		ResizedFilePath: thumbUpload.Location,
		ResizeParam:     inputResize,
	}}, inputFile)
	if err != nil {
		return operations.NewResizeInternalServerError().WithPayload(&models.Error{Detail: err.Error()})
	}

	return operations.NewResizeExistsOK().WithPayload(&models.Resize{
		Original: fileInfo.FilePath,
		Resized:  thumbUpload.Location,
	})
}
