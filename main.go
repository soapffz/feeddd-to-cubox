package main

import (
	"bytes"
	"encoding/json"
	"feeddd-to-cubox/localutils"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	mapset "github.com/deckarep/golang-set/v2"
	"github.com/soapffz/common-go-functions/pkg"
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
	// 记录程序运行时间
	location, _ := time.LoadLocation("Asia/Shanghai")
	startTime := time.Now().In(location)

	// 指定的时间段，默认一个小时请求一次
	contrabseconds := 3600 * 1
	// log.Println("当前时间为：" + time.Now().Format("2006-01-02 15:04:05"))
	// log.Println("当前的UTC时间为：" + time.Now().UTC().Format("2006-01-02 15:04:05"))
	m, _ := time.ParseDuration("1s")
	fromTime := time.Now().UTC().Add(-m * time.Duration(contrabseconds))

	// 初始化配置信息
	cuboxApi := viper.GetString("cubox.api")          //api
	defaulttag := viper.GetString("cubox.defaulttag") //默认标签
	threads := viper.GetInt("cubox.threads")          //并发数
	if cuboxApi == "" || cuboxApi == "sdasdasda" {
		log.Println("[-] 请填写cubox的api")
		os.Exit(0)
	}
	// 公众号黑名单处理
	blwxpbcount := viper.GetString("cubox.blwxpbcount")
	tmp1list := strings.Split(blwxpbcount, ",")
	blPbCountSet := mapset.NewSet[string]()
	for _, v := range tmp1list {
		blPbCountSet.Add(v)
	}

	// 公众号关键词黑名单处理
	blPbCountKwstrings := viper.GetString("cubox.blwxpbcountkeywords")
	blPbCountKwl := strings.Split(blPbCountKwstrings, ",")
	blPbCountFilter := localutils.NewTrie(blPbCountKwl)

	// 文章标题关键词黑名单处理
	blArticleKwstrings := viper.GetString("cubox.blwxarticlekeywords")
	blArticleKwl := strings.Split(blArticleKwstrings, ",")
	blArticleFilter := localutils.NewTrie(blArticleKwl)

	// 获取所有的rss链接
	allRssUrlList := GetAllRssUrlFromFedddd(blPbCountSet, blPbCountFilter)
	if allRssUrlList != nil {
		log.Println("[+] 共获取到公众号的rss订阅链接数：" + strconv.Itoa(len(allRssUrlList)))
		log.Println("[+] 解析时间范围：" + strconv.Itoa(contrabseconds/60) + "分钟内")

		// 解析指定时间内所有文章的链接及标题推送至cubox
		articleCount := GetArticleLAndPushToCubox(allRssUrlList, fromTime, cuboxApi, defaulttag, threads, blArticleFilter)
		if articleCount > 0 {
			log.Println("[+] 解析完成，过去的" + strconv.Itoa(contrabseconds/60) + "分钟内解析出" + strconv.Itoa(articleCount) + "篇文章")
		} else {
			log.Println("[+] 过去的" + strconv.Itoa(contrabseconds/60) + "分钟内没有更新的文章")
		}
	}
	endTime := time.Now().In(location)
	log.Println("[+] 程序运行时间：" + endTime.Sub(startTime).String())
}

func GetArticleLAndPushToCubox(allRssUrlList []string, fromTime time.Time, cuboxApi string, defaulttag string, threads int, blArticleFilter localutils.Trie) int {
	// 获取指定时间段内的文章链接，传入开始时间及rss链接列表

	var wg sync.WaitGroup
	if threads > 1000 || threads < 1 {
		log.Println("[-] 指定的线程数不合法，给你默认设置为2")
		threads = 2
	}
	log.Println("[+] 线程数为：" + strconv.Itoa(threads))
	ch := make(chan string, threads)

	// 记录文章篇数
	articleCount := 0
	blArticleNum := 0

	// 收集文章标题用于后续分析，优化黑名单
	articleTitleList := []string{}

	for _, eachrssurl := range allRssUrlList {
		// 解析每个rss订阅源
		ch <- eachrssurl
		wg.Add(1)
		go func(eachrssurl string) {
			defer wg.Done()
			res, err := http.Get(eachrssurl)
			if err != nil {
				log.Println("[-] 获取rss订阅源失败：" + eachrssurl)

			}
			defer res.Body.Close()
			if res.StatusCode == 200 {
				body, _ := io.ReadAll(res.Body)
				rs := string(body)
				if rs == "" {
					log.Println("[-] rss订阅源为空：" + eachrssurl)
					return
				}
				feedddJsonData := FeedddJson{}
				err := json.Unmarshal([]byte(rs), &feedddJsonData)
				if err != nil {
					log.Fatal(err)
				}

				feedddItems := feedddJsonData.Items
				for _, eachitem := range feedddItems {
					// 获取每个文章的发布时间
					articleTime := eachitem.DateModified.UTC()
					// log.Println(articleTime)
					if articleTime.After(fromTime) {
						// 打印文章信息，时间输出为中国时间
						// location, _ := time.LoadLocation("Asia/Shanghai")
						// articleTime.In(location).Format("2006-01-02 15:04:05")

						// 文章标题黑名单解析
						articleTitle := eachitem.Title
						keyWordsL := blArticleFilter.FindKeywords(articleTitle)
						if len(keyWordsL) > 0 {
							// log.Println("[-] 公众号【" + feedddJsonData.Title + "】 的文章【" + articleTitle + "】标题在关键词黑名单中，跳过")
							blArticleNum++
							continue
						}

						// log.Println("[+] 获取到公众号【" + feedddJsonData.Title + "】在【" + articleTime.Format("2006-01-02 15:04:05") + "】更新的文章 【" + eachitem.Title + " 】")
						articleTitleList = append(articleTitleList, articleTitle)

						articleCount++

						// if len(eachitem.URL) < 256 {
						// 	// 进行推送
						// 	PushDataToCubox(eachitem.URL, eachitem.Title, cuboxApi)
						// }
					}
				}
			}
			<-ch // 出队列
		}(eachrssurl)
	}
	wg.Wait()
	close(ch)
	if articleCount > 0 {
		log.Println("[+] 公众号文章标题关键词黑名单命中数量:" + strconv.Itoa(blArticleNum))
		log.Println("[+] 本次共解析得到文章数量:" + strconv.Itoa(articleCount))
	}
	pkg.WriteSliceReturnRandomFilename(articleTitleList)
	return articleCount // 返回文章篇数
}

func PushDataToCubox(url string, title string, cuboxApi string, defaulttag string) {
	cuboxApiUrl := "https://cubox.pro/c/api/save/" + cuboxApi

	// 构造json数据
	postData := make(map[string]string)
	postData["type"] = "url"
	postData["content"] = url
	postData["title"] = title
	postData["tag"] = defaulttag

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

func GetAllRssUrlFromFedddd(blPbCountSet mapset.Set[string], blPbCountFilter localutils.Trie) []string {
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

		// 公众号黑名单个数
		blPbCountNum := 0
		// 公众号黑名单关键词命中个数
		blPbCountNameNum := 0
		// 收集公众号名称用于后续黑名单优化操作
		var pbCountNameList []string

		for _, line := range linel {
			if line != "" {
				tmpl := strings.Split(line, " ")
				if len(tmpl) == 2 {
					pbCountName := tmpl[1]

					// 公众号名称黑名单解析
					if blPbCountSet.Contains(pbCountName) {
						blPbCountNum++
						// log.Println("[-] 公众号【" + pbCountName + "】在黑名单中，跳过")
						continue
					}

					// 公众号关键词黑名单解析
					keyWordsL := blPbCountFilter.FindKeywords(pbCountName)
					if len(keyWordsL) > 0 {
						blPbCountNameNum++
						// log.Println("[-] 公众号【" + pbCountName + "】在关键词黑名单中，跳过")
						continue
					}
					pbCountNameList = append(pbCountNameList, pbCountName)
					wxRssUrlList = append(wxRssUrlList, tmpl[0])
				}
			}
		}
		// wg.Wait() // 等待所有协程执行完毕
		if len(wxRssUrlList) > 0 {
			log.Println("[+] 公众号黑名单命中个数：" + strconv.Itoa(blPbCountNum))
			log.Println("[+] 公众号关键词黑名单命中个数：" + strconv.Itoa(blPbCountNameNum))
			pkg.WriteSliceReturnRandomFilename(pbCountNameList)
			return wxRssUrlList
		} else {
			log.Println("[-] 未获取到任何公众号rss订阅链接")
			os.Exit(1)
		}
	}
	return nil
}
