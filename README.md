## roller

### 概述

roller提供文件写入滚动，并对写入的文件按照策略进行轮换，生命周期管理（按照文件个数，总文件夹大小，保留时间），整体项目是基于 github.com/natefinch/lumberjack 进行修改，扩展相关功能

### 快速开始

```golang
func main() {
	r, err := roller.NewLumberjackRoller(
		roller.FileName("./data/test.data"),
		roller.FileMaxCount(10),
		roller.MaxSize(30),
		roller.FileMaxAge(time.Hour*24*7),
		roller.WithRotateStrategy(roller.DirectRotateStrategy),
		roller.FileMaxSize(0),
	)
	if err != nil {
		log.Fatalln(err)
	}
	ch := make(chan os.Signal)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM, syscall.SIGKILL)
	t := time.NewTicker(time.Second)
	for {
		select {
		case <-ch:
			log.Println("program exit: ")
			close(ch)
			r.Close()
			return
		case <-t.C:
			if _, err := r.Write([]byte("hello\n")); err != nil {
				log.Fatalln(err)
			}
		}
	}
}
```

### 功能介绍

- 文件轮换策略
    - 支持按照文件大小轮换
    - 支持每次写入直接轮换
- 文件保留策略（多个条件若指定了，则必须都满足）
    - 支持设置文件保留时间
    - 支持设置文件保留数量
    - 支持设置文件保留
- 支持文件压缩
    - gzip

### 使用说明

- 基于文件大小来轮换的，每次停止为了让这个日志能够被收集到，需要手动调用Rotate，然后在调用Close
- 每次写入后直接轮换的，每次停止时，直接调用close就可以了