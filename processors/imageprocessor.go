package processors

import (
	"errors"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	"github.com/xan-mortum/apimediaservice/components/imagemanager"
	"github.com/xan-mortum/apimediaservice/interfaces"
	"github.com/xan-mortum/apimediaservice/repositories"
)

//штука которая асинхронно обрабатывает файлы
type ImageProcessor struct {
	done             chan bool
	getTasksIn       chan ResizeTask
	Logger           interfaces.Logger
	taskRepository   *repositories.TaskRepository
	resizeRepository *repositories.ResizeRepository
	imageRepository  *repositories.ImageRepository
	s3Session        *session.Session
	im               imagemanager.ImageManager
	bucket           string
}

type ResizeTask struct {
	UUID   string
	Image  string
	Resize uint
}

func NewImageProcessor(
	logger interfaces.Logger,
	tr *repositories.TaskRepository,
	rr *repositories.ResizeRepository,
	ir *repositories.ImageRepository,
	s3 *session.Session,
	im imagemanager.ImageManager,
	bucket string,
) *ImageProcessor {
	return &ImageProcessor{
		done:             make(chan bool),
		getTasksIn:       make(chan ResizeTask, 100),
		Logger:           logger,
		taskRepository:   tr,
		resizeRepository: rr,
		imageRepository:  ir,
		s3Session:        s3,
		im:               im,
		bucket:           bucket,
	}
}

func (ip *ImageProcessor) Start() {
	go func() {
		for {
			select {
			case task := <-ip.getTasksIn:
				ip.runTusk(task)
			case <-ip.done:
				return
			}
		}
	}()
}

func (ip *ImageProcessor) Stop() {
	go func() {
		ip.done <- true
	}()
}

func (ip *ImageProcessor) AddTask(task ResizeTask) error {
	err := ip.taskRepository.Put(repositories.Task{
		Status: repositories.StatusInProgress,
	}, task.UUID)
	if err != nil {
		return err
	}

	wait := make(chan bool)
	go func() {
		ip.getTasksIn <- task
		close(wait)
	}()
	<-wait
	return nil
}

func (ip *ImageProcessor) GetTask(taskId string) (*repositories.Task, error) {
	dbTask, err := ip.taskRepository.Get(taskId)
	if err != nil {
		return nil, err
	}
	return dbTask, nil
}

func (ip *ImageProcessor) runTusk(task ResizeTask) {
	//скачиваем картинку с S3
	buf := aws.NewWriteAtBuffer([]byte{})
	downloader := s3manager.NewDownloader(ip.s3Session)
	_, err := downloader.Download(buf, &s3.GetObjectInput{
		Bucket: aws.String(ip.bucket),
		Key:    aws.String(task.Image),
	})
	if err != nil {
		ip.handleError(err, task.UUID)
	}

	//сохраняем файл во временную папку
	downloadedFile, err := ip.im.SaveFile(task.Image, buf.Bytes())
	if err != nil {
		ip.handleError(err, task.UUID)
	}

	//ресайзим картинку
	thumbFile, err := ip.im.ResizeFile(downloadedFile, task.Resize)
	if err != nil {
		ip.handleError(err, task.UUID)
	}
	if thumbFile == nil {
		ip.handleError(errors.New("new file is not created"), task.UUID)
	}

	//получаем ссылку на файл
	thumbToUpload, err := ip.im.GetFileResource(thumbFile)
	defer func() {
		err := thumbToUpload.Close()
		if err != nil {
			ip.handleError(err, task.UUID)
		}
	}()
	if err != nil {
		ip.handleError(err, task.UUID)
	}

	//загруаем на S3
	uploader := s3manager.NewUploader(ip.s3Session)
	thumbUpload, err := uploader.Upload(&s3manager.UploadInput{
		Bucket: aws.String(ip.bucket),
		Key:    aws.String(thumbFile.Name),
		Body:   thumbToUpload,
	})
	if err != nil {
		ip.handleError(err, task.UUID)
	}

	//чистим папку
	ip.im.Clear()

	//сохраняем в базу
	err = ip.resizeRepository.Append([]repositories.ImageResizeInfo{{
		ResizedFileName: thumbFile.Name,
		ResizedFilePath: thumbUpload.Location,
		ResizeParam:     int64(task.Resize),
	}}, task.Image)
	if err != nil {
		ip.handleError(err, task.UUID)
	}

	image, err := ip.imageRepository.Get(task.Image)
	if err != nil {
		ip.handleError(err, task.UUID)
	}

	dbTask, err := ip.taskRepository.Get(task.UUID)
	if err != nil {
		ip.handleError(err, task.UUID)
	}

	dbTask.Status = repositories.StatusDone
	dbTask.FilePath = image.FilePath
	dbTask.FileName = task.Image
	dbTask.ResizedFilePath = thumbUpload.Location
	dbTask.ResizedFileName = thumbFile.Name

	err = ip.taskRepository.Put(*dbTask, task.UUID)
	if err != nil {
		ip.handleError(err, task.UUID)
	}
}

func (ip *ImageProcessor) handleError(inErr error, taskId string) {
	dbTask, err := ip.taskRepository.Get(taskId)
	if err != nil {
		ip.Logger.Warning(err)
	}

	dbTask.Status = repositories.StatusError
	dbTask.Error = inErr.Error()

	err = ip.taskRepository.Put(*dbTask, taskId)
	if err != nil {
		ip.Logger.Warning(err)
	}
}
