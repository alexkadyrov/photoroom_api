package main

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	_ "time"

	"github.com/fsnotify/fsnotify"
	"gopkg.in/yaml.v3"
)

const (
	sourceDir    = "./source"
	destDir      = "./destination"
	processedDir = "./processed"
	configPath   = "config.yaml"
)

type Config struct {
	APIUrl string `yaml:"api_url"`
	APIKey string `yaml:"api_key"`
}

var config *Config

func main() {
	var err error
	config, err = loadConfig(configPath)
	if err != nil {
		log.Fatalf("Ошибка чтения конфигурации: %v", err)
	}

	// Создаем директории, если они не существуют
	createDirIfNotExists(sourceDir)
	createDirIfNotExists(destDir)
	createDirIfNotExists(processedDir)
	dirWatcher()
}

// Функция для загрузки конфигурации из файла
func loadConfig(path string) (*Config, error) {
	var config Config

	yamlFile, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	err = yaml.Unmarshal(yamlFile, &config)
	if err != nil {
		return nil, err
	}

	return &config, nil
}

func dirWatcher() {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Fatal(err)
	}
	defer watcher.Close()

	done := make(chan bool)
	go func() {
		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				if event.Op&fsnotify.Create == fsnotify.Create {
					filePath := event.Name
					if !isDirectory(filePath) {
						err := processFile(filePath)
						if err != nil {
							log.Println("Ошибка обработки файла:", err)
						} else {
							moveFile(filePath, destDir)
						}
					}
				}
			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				log.Println("error:", err)
			}
		}
	}()

	err = watcher.Add(sourceDir)
	if err != nil {
		log.Fatal(err)
	}

	<-done
}

func createDirIfNotExists(dir string) {
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		err := os.MkdirAll(dir, os.ModePerm)
		if err != nil {
			log.Fatal(err)
		}
	}
}

func isDirectory(path string) bool {
	fileInfo, err := os.Stat(path)
	if err != nil {
		log.Println("error:", err)
		return false
	}
	return fileInfo.IsDir()
}

func moveFile(src string, destDir string) {
	err := exec.Command("mv", src, destDir).Run()
	if err != nil {
		log.Println("error moving file:", err)
		return
	}
	fmt.Printf("File %s moved to %s\n", filepath.Base(src), destDir)
}

func processFile(filePath string) error {
	log.Println("process file:", filePath)

	pathParts := strings.Split(filePath, "/")

	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("не удалось открыть файл: %w", err)
	}
	defer file.Close()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	part, err := writer.CreateFormFile("imageFile", filepath.Base(filePath))
	if err != nil {
		return fmt.Errorf("не удалось создать форм-дату часть: %w", err)
	}

	_, err = io.Copy(part, file)
	if err != nil {
		return fmt.Errorf("ошибка при копировании данных файла: %w", err)
	}
	_ = writer.WriteField("background.color", "FFFFFF")
	err = writer.Close()
	if err != nil {
		return err
	}

	contentType := writer.FormDataContentType()
	writer.Close()

	req, err := http.NewRequest(http.MethodPost, config.APIUrl, body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", contentType)
	req.Header.Add("x-api-key", config.APIKey)

	client := &http.Client{}
	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if res.StatusCode != 200 {
		body, err := io.ReadAll(res.Body)
		if err != nil {
			return fmt.Errorf("ошибка при ReadAll: %w", err)
		}

		return fmt.Errorf("ошибка при получении ответа: %s", string(body))
	}

	respBody, err := io.ReadAll(res.Body)
	if err != nil {
		return fmt.Errorf("ошибка при ReadAll: %w", err)
	}

	// Открываем файл для записи. Если файл не существует, он будет создан.
	file, err = os.OpenFile(processedDir+"/"+pathParts[len(pathParts)-1], os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("Ошибка при открытии файла:", err)
	}
	defer file.Close()

	// Записываем данные в файл
	_, err = file.Write(respBody)
	if err != nil {
		return fmt.Errorf("Ошибка при записи в файл:", err)
	}

	return nil
}
