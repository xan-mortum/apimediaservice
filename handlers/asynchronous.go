package handlers

import (
	"github.com/go-openapi/runtime/middleware"
	"github.com/google/uuid"
	"github.com/xan-mortum/apimediaservice/gen/models"
	"github.com/xan-mortum/apimediaservice/gen/restapi/operations"
	"github.com/xan-mortum/apimediaservice/interfaces"
	"github.com/xan-mortum/apimediaservice/processors"
	"github.com/xan-mortum/apimediaservice/repositories"
)

type AsynchronousHandler struct {
	Logger              interfaces.Logger
	ImageProcessor      *processors.ImageProcessor
	UserImageRepository *repositories.UserImageRepository
	ResizeRepository    *repositories.ResizeRepository
}

func NewAsynchronousHandler(
	logger interfaces.Logger,
	ip *processors.ImageProcessor,
	userImageRepository *repositories.UserImageRepository,
	resizeRepository *repositories.ResizeRepository,
) *AsynchronousHandler {
	return &AsynchronousHandler{
		Logger: logger,
		ImageProcessor:      ip,
		UserImageRepository: userImageRepository,
		ResizeRepository:    resizeRepository,
	}
}

func (handler *AsynchronousHandler) V2resizeHandler(params operations.V2resizeParams) middleware.Responder {
	inputResize := params.Resize

	id := uuid.New().String()
	task := processors.ResizeTask{
		UUID:   id,
		Image:  params.File,
		Resize: uint(inputResize),
	}

	err := handler.ImageProcessor.AddTask(task)
	if err != nil {
		return operations.NewV2resizeInternalServerError().WithPayload(&models.Error{Detail: err.Error()})
	}
	return operations.NewV2resizeOK().WithPayload(id)
}

func (handler *AsynchronousHandler) ResultHandler(params operations.ResultParams) middleware.Responder {
	taskId := params.Execution

	task, err := handler.ImageProcessor.GetTask(taskId)
	if task == nil {
		return operations.NewResultBadRequest().WithPayload(&models.Error{Detail: "execution " + taskId + " not found"})
	}
	if err != nil {
		return operations.NewResultInternalServerError().WithPayload(&models.Error{Detail: err.Error()})
	}

	return operations.NewResultOK().WithPayload(&models.Resize{
		Original: task.FilePath,
		Resized:  task.ResizedFilePath,
	})
}

func (handler *AsynchronousHandler) V2filesHandler(params operations.V2filesParams) middleware.Responder {
	//по этому токену определяем пользователя. писать можно любую стоку при ресайзе, и потом ее же присылать сюда
	inputToken := params.Token

	//получаем из базы информацию о картинках пользователя
	files, err := handler.UserImageRepository.Get(inputToken)
	if err != nil {
		return operations.NewV2filesBadRequest().WithPayload(&models.Error{Detail: err.Error()})
	}

	//получаем резайзы картинок
	var result []repositories.UserImage
	for _, file := range files {
		resizeInfo, err := handler.ResizeRepository.Get(file.OriginalFileName)
		if err != nil {
			return operations.NewV2filesBadRequest().WithPayload(&models.Error{Detail: err.Error()})
		}
		file.Resized = append(file.Resized, resizeInfo...)
		result = append(result, file)
	}

	return operations.NewV2filesOK().WithPayload(result)
}
