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
	startTime := time.Now()

	// 初始化配置信息
	cuboxApi := viper.GetString("cubox.api")                 //api
	defaulttagcontent := viper.GetString("cubox.defaulttag") //默认标签
	defaulttag := []string{defaulttagcontent}                // 标签处理为数组
	threads := viper.GetInt("cubox.threads")                 //并发数
	if cuboxApi == "" || cuboxApi == "sdasdasda" {
		log.Println("[-] 请填写cubox的api")
		os.Exit(0)
	}
	blKeyWords := viper.GetString("cubox.blkeywords")    //黑名单关键词
	peroidminutes := viper.GetInt("cubox.peroidminutes") //获取的文章时间间隔
	if peroidminutes > 1440 || peroidminutes < 1 {
		log.Println("[-] 指定的线程数不合法，给你默认设置为60")
		threads = 60
	}
	// 指定的时间段，默认一个小时请求一次
	contrabseconds := 60 * peroidminutes
	// log.Println("当前时间为：" + time.Now().Format("2006-01-02 15:04:05"))
	// log.Println("当前的UTC时间为：" + time.Now().UTC().Format("2006-01-02 15:04:05"))
	m, _ := time.ParseDuration("1s")
	fromTime := time.Now().UTC().Add(-m * time.Duration(contrabseconds))

	// 黑名单关键词处理
	blKeyWordsL := strings.Split(blKeyWords, ",")
	blKeyWordsFilter := localutils.NewTrie(blKeyWordsL)

	// 获取所有的rss链接
	allRssUrlList := GetAllRssUrlFromFedddd(blKeyWordsFilter)
	if allRssUrlList != nil {
		log.Println("[+] 解析时间范围：" + strconv.Itoa(contrabseconds/60) + "分钟内")
		log.Println("[+] 共获取到公众号的rss订阅链接数：" + strconv.Itoa(len(allRssUrlList)))

		// 解析指定时间内所有文章的链接及标题推送至cubox
		articleCount := GetArticleLAndPushToCubox(allRssUrlList, fromTime, cuboxApi, defaulttag, threads, blKeyWordsFilter)
		if articleCount == 0 {
			log.Println("[+] 过去的" + strconv.Itoa(contrabseconds/60) + "分钟内没有更新的文章")
		}
	}
	endTime := time.Now()
	log.Println("[+] 程序运行时间：" + endTime.Sub(startTime).String())
}

func GetArticleLAndPushToCubox(allRssUrlList []string, fromTime time.Time, cuboxApi string, defaulttag []string, threads int, blKeyWordsFilter localutils.Trie) int {
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
	pushSuccNum := 0

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
					// log.Println("[+] 文章发布时间：" + articleTime.Format("2006-01-02 15:04:05"))
					// log.Println(articleTime)
					if articleTime.After(fromTime) {
						// 打印文章信息，时间输出为中国时间
						// location, _ := time.LoadLocation("Asia/Shanghai")
						// articleTime.In(location).Format("2006-01-02 15:04:05")

						// 文章标题黑名单解析
						articleTitle := eachitem.Title
						matchedKeyWordsL := blKeyWordsFilter.FindKeywords(articleTitle)
						if len(matchedKeyWordsL) > 0 {
							// log.Println("[-] 公众号【" + feedddJsonData.Title + "】 的文章【" + articleTitle + "】标题在关键词黑名单中，跳过")
							blArticleNum++
							continue
						}

						// log.Println("[+] 获取到公众号【" + feedddJsonData.Title + "】在【" + articleTime.Format("2006-01-02 15:04:05") + "】更新的文章 【" + eachitem.Title + " 】")
						articleTitleList = append(articleTitleList, articleTitle)
						articleCount++

						if len(eachitem.URL) < 256 {
							// 进行推送
							pushSuccFlag := PushContentToCubox(eachitem.URL, eachitem.Title, cuboxApi, defaulttag)
							if pushSuccFlag {
								pushSuccNum++
							}
						}
					}
				}
			} else {
				log.Println("[-] rss订阅源返回状态码为:" + strconv.Itoa(res.StatusCode) + ":" + eachrssurl)
				return
			}
			<-ch // 出队列
		}(eachrssurl)
	}
	wg.Wait()
	close(ch)
	if articleCount > 0 {
		log.Println("[+] 公众号文章标题关键词黑名单命中数量:" + strconv.Itoa(blArticleNum))
		log.Println("[+] 本次共解析得到文章数量:" + strconv.Itoa(articleCount))
		log.Println("[+] 本次推送Cubox成功数量:" + strconv.Itoa(pushSuccNum))
	}
	// pkg.WriteSliceReturnRandomFilename(articleTitleList)
	return articleCount // 返回文章篇数
}

func PushContentToCubox(url string, title string, cuboxApi string, defaulttag []string) bool {
	cuboxApiUrl := "https://cubox.pro/c/api/save/" + cuboxApi

	// 构造json数据
	postData := make(map[string]interface{})
	postData["type"] = "url"
	postData["content"] = url
	postData["title"] = title
	postData["tags"] = defaulttag

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
			return false
		}
		if cbRespData.Code == -1 {
			// log.Println("[-] 推送失败：" + cbRespData.Message)
			return false
		} else {
			// log.Println("[+] 文章推送成功：【" + title + "】【" + url + "】")
			return true
		}
	}
	return false
}

func GetAllRssUrlFromFedddd(blKeyWordsFilter localutils.Trie) []string {
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

		// 公众号黑名单关键词命中个数
		blPbCountNum := 0
		// 收集公众号名称用于后续黑名单优化操作
		var pbCountNameList []string

		for _, line := range linel {
			if line != "" {
				tmpl := strings.Split(line, " ")
				if len(tmpl) == 2 {
					pbCountName := tmpl[1]

					// 公众号名称黑名单解析
					matchedKeyWordsL := blKeyWordsFilter.FindKeywords(pbCountName)
					if len(matchedKeyWordsL) > 0 {
						blPbCountNum++
						// log.Println("[-] 公众号【" + pbCountName + "】在黑名单中，跳过")
						continue
					}

					pbCountNameList = append(pbCountNameList, pbCountName)
					wxRssUrlList = append(wxRssUrlList, tmpl[0])
				}
			}
		}
		// wg.Wait() // 等待所有协程执行完毕
		if len(wxRssUrlList) > 0 {
			log.Println("[+] 公众号名称黑名单关键词命中个数：" + strconv.Itoa(blPbCountNum))
			// pkg.WriteSliceReturnRandomFilename(pbCountNameList)
			return wxRssUrlList
		} else {
			log.Println("[-] 未获取到任何公众号rss订阅链接")
			os.Exit(1)
		}
	}
	return nil
}
