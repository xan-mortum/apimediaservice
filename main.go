package main

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/go-openapi/loads"
	"github.com/op/go-logging"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/xan-mortum/apimediaservice/components/imagemanager"
	"github.com/xan-mortum/apimediaservice/gen/restapi"
	"github.com/xan-mortum/apimediaservice/gen/restapi/operations"
	"github.com/xan-mortum/apimediaservice/handlers"
	"github.com/xan-mortum/apimediaservice/processors"
	"github.com/xan-mortum/apimediaservice/repositories"
	"os"
)

const Port = 8085
const Region = "us-east-2"
const ID = "ID"
const Secret = "Secret"
const Bucket = "xan.resizeimage"

var log = logging.MustGetLogger("apimediaservice")
var format = logging.MustStringFormatter(
	`%{color}%{time:15:04:05.000} %{shortfunc} ▶ %{level:.4s} %{id:03x}%{color:reset} %{message}`,
)

func main() {
	file, err := os.Create("log.log")
	if err != nil {
		log.Fatal(err)
	}
	backendFile := logging.NewLogBackend(file, "", 0)
	backendStdin := logging.NewLogBackend(os.Stdin, "", 0)

	backend2Formatter := logging.NewBackendFormatter(backendStdin, format)

	backend1Leveled := logging.AddModuleLevel(backendFile)
	backend1Leveled.SetLevel(logging.DEBUG, "")

	logging.SetBackend(backend1Leveled, backend2Formatter)

	//первая попавшаяся под руку база которую не надо отдельно устанавливать
	//для того что бы начать использовать другую базу нужно будет изменить репозитории
	//и исползование репозиториев в коде
	//на то что бы инкапсулировать это нужно потратить довольно много времени
	db, err := leveldb.OpenFile("db", nil)
	if err != nil {
		log.Debug("open db")
		log.Fatal(err)
	}
	defer func() {
		err := db.Close()
		if err != nil {
			log.Fatal(err)
		}
	}()

	swaggerSpec, err := loads.Analyzed(restapi.SwaggerJSON, "")
	if err != nil {
		log.Fatal(err)
	}
	api := operations.NewApimediaserviceAPI(swaggerSpec)
	server := restapi.NewServer(api)
	defer func() {
		err := server.Shutdown()
		if err != nil {
			log.Fatal(err)
		}
	}()

	imageManagerConfig := imagemanager.Config{TmpDir: "./tmp/"}
	imageManager := imagemanager.NewImageManager(imageManagerConfig)

	sess, err := session.NewSession(
		&aws.Config{
			Region:      aws.String(Region),
			Credentials: credentials.NewStaticCredentials(ID, Secret, ""),
		},
	)
	if err != nil {
		log.Fatal(err)
	}

	userImageRepository := repositories.NewUserImageRepository(db)
	imageRepository := repositories.NewImageRepository(db)
	resizeRepository := repositories.NewResizeRepository(db)
	taskRepository := repositories.NewTaskRepository(db)

	//тут храняться хандлеры которых не должно быть вообще. то есть, созданные только для этого
	mockHandler := handlers.NewMockHandler(
		log,
		imageManager,
		sess,
		userImageRepository,
		imageRepository,
	)

	//документацию по апи можно посмотреть выполнив make serve-swagger из корня проекта
	//там же можно и отправить запросы. но для этого нужно будет запустить сервис
	//для этого нужно вызвать make build и после этого make start

	//тут реализация апи с задания
	//решение плохое потому что при хоть какой то нахрузке начнет выдвать отказы
	//так же, никогда не видел что бы файл загружался на сервер вместе с данными
	//
	//POST http://localhost:8085/v1/resize_exists
	//Content-Type: multipart/form-data
	//параметры формы:
	//token - стока. можно получить вызовом http://localhost:8085/token но не важно что туда писать. главное запомнить это значение
	//resize - число. указываеться размер картинки. реализовал только это и с одним параметром. расширять можно сколько угодно
	//
	//GET http://localhost:8085/v1/files?token={token}
	//возвращает все файлы то были загружены с указанным token
	//
	//POST http://localhost:8085/v1/resize_exists - ресайзит существующую картинку
	//Content-Type: multipart/form-data
	//параметры формы:
	//token - стока.
	//resize - число.
	//file - uuid файла. его можно получить в ответе вызова http://localhost:8085/v1/files?token={token}
	synchronousHandler := handlers.NewSynchronousHandler(
		log,
		imageManager,
		sess,
		userImageRepository,
		imageRepository,
		resizeRepository,
		Bucket,
	)

	api.TokenHandler = operations.TokenHandlerFunc(mockHandler.TokenHandler)
	api.ResizeHandler = operations.ResizeHandlerFunc(synchronousHandler.ResizeHandler)
	api.FilesHandler = operations.FilesHandlerFunc(synchronousHandler.FilesHandler)
	api.ResizeExistsHandler = operations.ResizeExistsHandlerFunc(synchronousHandler.ResizeExistsHandler)

	//дальше реализация второй версии апи. она асинхронная что бы не создавать нагрузку на сервер висящими коннектами
	//основная работа по манипуляциям с фото переложенна на этот процессор
	imageProcessor := processors.NewImageProcessor(
		log,
		taskRepository,
		resizeRepository,
		imageRepository,
		sess,
		imageManager,
		Bucket,
	)
	imageProcessor.Start()
	defer imageProcessor.Stop()

	//POST http://localhost:8085/v2/upload?token={token}- загрузка файла на сервер
	//обычно, файлы на s3 грузяться с клиента и на сервер отправляеться уже ссылка на файл
	//иначе с s3 толку никакого нет
	//параметр один
	//upfile - это файл
	//возвращаеться имя этого файла которе выполняет роль идентификатора
	//
	//POST http://localhost:8085/v2/resize - отправляет файл на изменение размера
	//возвращаеться идентификатор задачи по которому потом можно получить результат. в том числе и ошибку
	//параметры:
	//token - сторка
	//file - строка которую вернул upload
	//resize - число
	//
	//http://localhost:8085/v2/result?token={token}&execution={uuid}
	//получаем результат
	//execution - это uuid задачи который возвращал предыдущий вызов
	//http://localhost:8085/v2/files?token={token} - получаем список файлов по токену
	asynchronousHandler := handlers.NewAsynchronousHandler(
		log,
		imageProcessor,
		userImageRepository,
		resizeRepository,
	)

	api.UploadHandler = operations.UploadHandlerFunc(mockHandler.UploadHandler)
	api.V2resizeHandler = operations.V2resizeHandlerFunc(asynchronousHandler.V2resizeHandler)
	api.ResultHandler = operations.ResultHandlerFunc(asynchronousHandler.ResultHandler)
	api.V2filesHandler = operations.V2filesHandlerFunc(asynchronousHandler.V2filesHandler)

	server.Port = Port
	err = server.Serve()
	if err != nil {
		log.Fatal(err)
	}
}
