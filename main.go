package main

import (
	"bytes"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/spf13/viper"
	//"github.com/wr69/mygotools"
	"github.com/wr69/mygotools/fileplus"
	"github.com/wr69/mygotools/notice"
)

var Error_Log *log.Logger
var Error_LogFile *os.File

func ResetErrorLogOutput() {
	logname := "log/" + "log_" + time.Now().Format("2006-01-02") + ".txt"
	err := os.MkdirAll(filepath.Dir(logname), 0755) //
	if err != nil {
		log.Println("无法创建目录：", err)
	}
	logFile, err := os.OpenFile(logname, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		log.Println("无法打开新的日志文件：", err)
	} else {
		if Error_LogFile != nil {
			defer Error_LogFile.Close()
		}
		Error_Log = log.New(logFile, "", log.Ldate|log.Ltime|log.Lshortfile)
		Error_Log.SetOutput(logFile)
		Error_LogFile = logFile
	}
}

func ErrorLog(v ...any) {
	if Error_Log == nil {
		ResetErrorLogOutput()
	}
	Error_Log.Println(v...)
}

func returnErr(v ...any) {
	log.Println(v...)
	os.Exit(1)
}

var CachePath = "cache/"
var NoticeKey string
var NoticeUrl string

func main() {
	log.Println("本次任务执行", "准备")
	var envConfig *viper.Viper

	getConfig := viper.New()

	getConfig.SetConfigName("auto-tongbudaozhiding")
	getConfig.SetConfigType("yaml")             // 如果配置文件的名称中没有扩展名，则需要配置此项
	getConfig.AddConfigPath(".")                // 还可以在工作目录中查找配置
	getConfig.AddConfigPath("../updata/config") // 还可以在工作目录中查找配置

	err := getConfig.ReadInConfig() // 查找并读取配置文件
	if err != nil {                 // 处理读取配置文件的错误
		//returnErr("读取配置文件的错误: ", err)
		envConfig = viper.New()
		envConfig.AutomaticEnv()
	} else {
		envConfig = getConfig.Sub("env") //读取env的配置
	}

	link := envConfig.GetString("LINK")
	if link == "" {
		returnErr("link配置不存在")
	}

	NoticeKey = envConfig.GetString("NOTICE_KEY")
	if NoticeKey == "" {
		returnErr("NOTICE_KEY 配置不存在")
	}

	NoticeUrl = envConfig.GetString("NOTICE_URL")
	if NoticeUrl == "" {
		returnErr("NOTICE_URL配置不存在")
	}

	log.Println("本次任务执行", "启动")

	results := make(chan string)
	limit := make(chan struct{}, 10) // 设置最大并发数为10

	groups := strings.Split(link, "\n")
	// 创建 WaitGroup，用于同步协程
	var wg sync.WaitGroup

	// 启动多个协程进行处理
	for _, group := range groups {
		wg.Add(1)

		go func(group string) {
			defer wg.Done()

			// 控制最大并发数
			limit <- struct{}{}

			// 执行具体的任务，并将结果发送到通道中
			result := processNumber(group)
			results <- result

			// 释放信号量
			<-limit
		}(group)

	}
	// 等待所有协程完成
	go func() {
		wg.Wait()
		close(results)
	}()

	// 从通道中读取结果
	postData := ""
	for result := range results {
		postData = postData + result
		//log.Println("result : ", result)
	}

	log.Println("result : ", postData)

	log.Println("本次任务执行", "结束")
}

func processNumber(group string) string {
	reStr := ""
	if group != "" {

		prop := strings.Split(group, "|")
		channelStr := prop[0]
		modeStr := prop[1]
		nameStr := prop[2]
		urlStr := prop[3]
		startRun(channelStr, modeStr, nameStr, urlStr)
	}
	return reStr
}

func startRun(channelStr string, modeStr string, nameStr string, urlStr string) {
	fileName := fileplus.UrlEscapeFilename(urlStr)
	filePath := CachePath + fileName

	// 获取网站内容
	StatusCode, websiteContent := getApi(urlStr)
	if StatusCode > 300 {
		ErrorLog(urlStr, "无法获取网站内容:", websiteContent)
		return
	}

	// 读取本地文件内容
	localContent, err := fileplus.GetFile(filePath)
	if err != nil {
		ErrorLog(urlStr, "无法读取本地文件", filePath, " 出错原因:", err)
		localContent = []byte("")
	}

	// 检查内容是否有更新
	if string(localContent) != websiteContent {

		go func(channel string, mode string, name string, postData string) {
			notice.Post(NoticeUrl, NoticeKey, channel, mode, name, postData)
		}(channelStr, modeStr, nameStr, websiteContent)

		err = fileplus.WriteFile(filePath, websiteContent)
		if err != nil {
			ErrorLog(urlStr, "内容已更新，无法写入本地文件:", err)
			return
		}
		log.Println(urlStr, "内容已更新，已写入文件缓存！")
	} else {
		log.Println(urlStr, "内容未更新")
	}
}

func getApi(apiurl string) (int, string) {
	StatusCode, resBody := reqApi("get", apiurl, http.Header{}, "")
	return StatusCode, resBody
}

func reqApi(method string, apiurl string, headers http.Header, data string) (int, string) {

	var req *http.Request
	var err error
	var dataSend io.Reader

	transport := &http.Transport{} // 创建自定义的 Transport

	proxy_url := "" //"http://127.0.0.1:8888"

	if proxy_url != "" {
		proxyURL, err := url.Parse(proxy_url) // 代理设置
		if err != nil {
			log.Println(apiurl, "代理出问题", err)
		} else {
			transport.Proxy = http.ProxyURL(proxyURL)
		}
	}

	// 创建客户端，并使用自定义的 Transport
	client := &http.Client{
		Timeout:   15 * time.Second, // 设置超时时间为10秒
		Transport: transport,        //
	}

	headers.Set("User-Agent", "Mozilla / 5.0 (Windows NT 10.0; Win64; x64) AppleWebKit / 537.36 (KHTML, like Gecko) Chrome / 114.0.0.0 Safari / 537.36 Edg / 114.0.1823.37")

	if strings.HasPrefix(data, "{") {
		dataSend = bytes.NewBuffer([]byte(data))
	} else {
		dataSend = strings.NewReader(data)
	}

	if method == "post" {
		req, err = http.NewRequest("POST", apiurl, dataSend)
	} else if method == "put" {
		req, err = http.NewRequest("PUT", apiurl, dataSend)
	} else if method == "delete" {
		req, err = http.NewRequest("DELETE", apiurl, nil)
	} else {
		req, err = http.NewRequest("GET", apiurl, nil)
	}
	if err != nil {
		return 9999, "http.NewRequest " + err.Error()
	}
	req.Header = headers

	resp, err := client.Do(req)
	if err != nil {
		return 9999, "client.Do " + err.Error()
	}
	defer resp.Body.Close()

	resBody, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, "io resp.Body " + err.Error()
	}

	return resp.StatusCode, string(resBody)

}
