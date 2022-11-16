package main

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/viper"
)

// 配置文件结构体
type Config struct {
	// 这里不一定需要所有配置都写全才能用viper读取
	Cubox CuboxConfig
}

type CuboxConfig struct {
	Api string
}

// feeddd提供的微信公众号订阅rss的内容对应的结构体
type FeedddJson struct {
	Version     string  `json:"version"`
	Title       string  `json:"title"`
	HomePageURL string  `json:"home_page_url"`
	FeedURL     string  `json:"feed_url"`
	Items       []Items `json:"items"`
}
type Items struct {
	ID           string    `json:"id"`
	URL          string    `json:"url"`
	Title        string    `json:"title"`
	Summary      string    `json:"summary"`
	DateModified time.Time `json:"date_modified"`
}

// Cubox 推送json格式结构体
type CuboxPostData struct {
	Type        string `json:"type" default:"url"`
	Content     string `json:"content"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Tags        string `json:"tags" default:"newview"`
	Folder      string `json:"folder"`
}

// 推送Cubox返回的json格式结构体
type CuboxRespData struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data"`
	List    interface{} `json:"list"`
}

func init() {
	// 初始化，viper读取配置文件
	var config Config
	viper.SetConfigName("config")
	viper.SetConfigType("toml")
	viper.AddConfigPath("configs")
	err := viper.ReadInConfig()
	if err != nil {
		log.Println(err)
		os.Exit(0)
	}
	viper.Unmarshal(&config) //将配置文件绑定到config上
}

func main() {
	// 指定的时间段，默认一个小时请求一次
	contrabseconds := 3600 * 1
	// log.Println("当前时间为：" + time.Now().Format("2006-01-02 15:04:05"))
	// log.Println("当前的UTC时间为：" + time.Now().UTC().Format("2006-01-02 15:04:05"))
	m, _ := time.ParseDuration("1s")
	fromTime := time.Now().UTC().Add(-m * time.Duration(contrabseconds))

	// // 处理提交时间，获取到的时间进行时区转换
	// // 提交时间先转换为时间格式
	// transCommitTime1, _ := time.Parse("2006-01-02 15:04:05", strings.Replace(string(getCommitTime_output), "\n", "", -1))
	// var cstSh, _ = time.LoadLocation("Asia/Shanghai") //上海
	// // 加上时区，会变成字符串，所以还需要再转换一次时间格式得到上海时区的提交时间
	// transCommitTime2 := transCommitTime1.In(cstSh).Format("2006-01-02 15:04:05")
	// commitTime, _ := time.Parse("2006-01-02 15:04:05", transCommitTime2)
	// addTime := commitTime.Add(time.Duration(xxljob_crontab_second) * m)

	// 初始化API信息
	cuboxApi := viper.GetString("cubox.api") //api
	if cuboxApi == "" || cuboxApi == "sdasdasda" {
		log.Println("[-] 请填写cubox的api")
		os.Exit(0)
	}
	allRssUrlList := GetAllRssUrlFromFedddd()
	if allRssUrlList != nil {
		log.Println("[+] 获取到" + strconv.Itoa(len(allRssUrlList)) + "个公众号的rss订阅链接")
		log.Println("[+] 开始解析" + strconv.Itoa(contrabseconds/60) + "分钟内更新的文章")

		// 解析指定时间内所有文章的链接及标题推送至cubox
		GetArticleLAndPushToCubox(allRssUrlList, fromTime, cuboxApi)

	}
}

func GetArticleLAndPushToCubox(allRssUrlList []string, fromTime time.Time, cuboxApi string) {
	// 获取指定时间段内的文章链接，传入开始时间及rss链接列表
	for _, eachrssurl := range allRssUrlList {
		// 解析每个rss订阅源
		res, err := http.Get(eachrssurl)
		if err != nil {
			log.Println("[-] 获取rss订阅源失败：" + eachrssurl)
			continue
		}
		defer res.Body.Close()
		if res.StatusCode == 200 {
			body, _ := io.ReadAll(res.Body)
			rs := string(body)
			if rs == "" {
				continue
			}
			feedddJsonData := FeedddJson{}
			err := json.Unmarshal([]byte(rs), &feedddJsonData)
			if err != nil {
				log.Fatal(err)
			}
			feedddItems := feedddJsonData.Items
			for _, eachitem := range feedddItems {
				// 获取每个文章的发布时间
				articleTime := eachitem.DateModified
				// log.Println(articleTime)
				if articleTime.After(fromTime) {
					// 打印文章信息，时间输出为中国时间
					location, _ := time.LoadLocation("Asia/Shanghai")
					log.Println("[+] 获取到公众号【" + feedddJsonData.Title + "】在【" + articleTime.In(location).Format("2006-01-02 15:04:05") + "】更新的文章 【" + eachitem.Title + " 】")

					// if len(eachitem.URL) < 256 {
					// 	// 进行推送
					// 	PushDataToCubox(eachitem.URL, eachitem.Title, cuboxApi)
					// }

				}
			}
			// os.Exit(0)
		}
	}
}

func PushDataToCubox(url string, title string, cuboxApi string) {
	cuboxApiUrl := "https://cubox.pro/c/api/save/" + cuboxApi

	// 构造json数据
	postData := make(map[string]string)
	postData["type"] = "url"
	postData["content"] = url
	postData["title"] = title

	// 发送请求
	b, _ := json.Marshal(postData)
	res, err := http.Post(cuboxApiUrl, "application/json", bytes.NewBuffer(b))
	if err != nil {
		log.Println("[-] 推送失败："+url, " ", err)
	}
	defer res.Body.Close()
	if res.StatusCode == 200 {
		tmpData, _ := io.ReadAll(res.Body)
		rs := string(tmpData)
		cbRespData := CuboxRespData{}
		err := json.Unmarshal([]byte(rs), &cbRespData)
		if err != nil {
			log.Fatal(err)
			return
		}
		if cbRespData.Code == -1 {
			log.Println("[-] 推送失败：" + cbRespData.Message)
		} else {
			log.Println("[+] 文章推送成功：【" + title + "】【" + url + "】")
		}
	}
}

func GetAllRssUrlFromFedddd() []string {
	// 从feeddd提供的定期更新链接中提取所有公众号的rss订阅链接
	feddddJsonUrl := "https://cdn.jsdelivr.net/gh/feeddd/feeds/feeds_all_json.txt"
	// 请求聚合链接并解析出url添加到队列中
	res, err := http.Get(feddddJsonUrl)
	if err != nil {
		log.Println()
		os.Exit(1)
	}
	defer res.Body.Close()
	if res.StatusCode == 200 {
		body, _ := io.ReadAll(res.Body)
		var wxRssUrlList []string
		allcontent := string(body)
		linel := strings.Split(allcontent, "\n")
		for _, line := range linel {
			if line != "" {
				tmpl := strings.Split(line, " ")
				if len(tmpl) == 2 {
					wxRssUrlList = append(wxRssUrlList, tmpl[0])
				}
			}

		}
		if len(wxRssUrlList) > 0 {
			return wxRssUrlList
		}
	}
	return nil
}
