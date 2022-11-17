# feeddd-to-cubox

解析 feeddd 到 cubox 中。Parsing feeddd into cubox.

# 功能

- 从[feeddd](https://github.com/feeddd/feeds)项目提供的链接中提取指定时间内更新的微信公众号文章，通过 API 发送至 Cubox 中进行收藏

  - 默认定时时间为 1 个小时，即提供链接中的时间在当前时间 1 个小时之内更新的文章，不是微信公众号发布时间 1 个小时之内，因为原项目本身就有延迟

  - 特别需要说明的是，因为 Cubox 高级版每日限定 200 条推送，所以建议一定要按照自己习惯设置黑名单关键词

# 使用方法

- 复制`configs/config-example.toml`里的文件为`configs/config.toml`，填写自己的配置之后直接使用即可

- 命令行无其他额外指定参数

# 程序效果

```
2022/11/17 22:23:36 [+] 公众号黑名单命中个数：8
2022/11/17 22:23:36 [+] 公众号关键词黑名单命中个数：3555
2022/11/17 22:23:36 [+] 共获取到公众号的rss订阅链接数：25771
2022/11/17 22:23:36 [+] 解析时间范围：60分钟内
2022/11/17 22:23:36 [+] 线程数为：100
2022/11/17 22:24:59 [+] 公众号文章标题关键词黑名单命中数量:29
2022/11/17 22:24:59 [+] 本次共解析得到文章数量:91
2022/11/17 22:24:59 [+] 本次推送Cubox成功数量:91
2022/11/17 22:24:59 [+] 程序运行时间：1m23.334126s
```

# 更新历史

- 2022 年 11 月 17 日晚

  - [update] 优化代码架构：使用 go 和 chan 实现协程，配置文件中可指定协程数默认为 100，超过 1000 或者小于 1 的话会被设置为 2

  - [add] 添加关键词及公众号黑名单过滤功能，公众号名称黑名单的全字匹配使用[deckarep/golang-set](https://github.com/deckarep/golang-set)库实现，其他关键字黑名单过滤使用[zeromicro/go-zero](https://github.com/zeromicro/go-zero)里面的组件[stringx](https://go-zero.dev/cn/docs/blog/tool/keywords/)实现

- 2022 年 11 月 17 日凌晨 1 点

  - [init] 写好第一版，已可以使用。