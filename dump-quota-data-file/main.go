package main

import (
	"context"
	"flag"
	"log"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/bryanchen463/quota_data_service/client/v1"
	"github.com/bryanchen463/quota_data_service/proto"
	"github.com/fsnotify/fsnotify"
	"gonum.org/v1/hdf5"
)

type QuoteData struct {
	ReceiveTimeStamp  int64
	ExchangeTimeStamp int64
	BestAskPrice      float64
	BestAskSize       float64
	BestBidPrice      float64
	BestBidSize       float64
	AskVolumesIn2k    float64
	BidVolumesIn2k    float64
	Ask1Price         float64
	Ask1Size          float64
	Ask2Price         float64
	Ask2Size          float64
	Ask3Price         float64
	Ask3Size          float64
	Ask4Price         float64
	Ask4Size          float64
	Ask5Price         float64
	Ask5Size          float64
	Ask6Price         float64
	Ask6Size          float64
	Ask7Price         float64
	Ask7Size          float64
	Ask8Price         float64
	Ask8Size          float64
	Ask9Price         float64
	Ask9Size          float64
	Ask10Price        float64
	Ask10Size         float64
	Ask11Price        float64
	Ask11Size         float64
	Ask12Price        float64
	Ask12Size         float64
	Ask13Price        float64
	Ask13Size         float64
	Ask14Price        float64
	Ask14Size         float64
	Ask15Price        float64
	Ask15Size         float64
	Ask16Price        float64
	Ask16Size         float64
	Ask17Price        float64
	Ask17Size         float64
	Ask18Price        float64
	Ask18Size         float64
	Ask19Price        float64
	Ask19Size         float64
	Ask20Price        float64
	Ask20Size         float64
	Bid1Price         float64
	Bid1Size          float64
	Bid2Price         float64
	Bid2Size          float64
	Bid3Price         float64
	Bid3Size          float64
	Bid4Price         float64
	Bid4Size          float64
	Bid5Price         float64
	Bid5Size          float64
	Bid6Price         float64
	Bid6Size          float64
	Bid7Price         float64
	Bid7Size          float64
	Bid8Price         float64
	Bid8Size          float64
	Bid9Price         float64
	Bid9Size          float64
	Bid10Price        float64
	Bid10Size         float64
	Bid11Price        float64
	Bid11Size         float64
	Bid12Price        float64
	Bid12Size         float64
	Bid13Price        float64
	Bid13Size         float64
	Bid14Price        float64
	Bid14Size         float64
	Bid15Price        float64
	Bid15Size         float64
	Bid16Price        float64
	Bid16Size         float64
	Bid17Price        float64
	Bid17Size         float64
	Bid18Price        float64
	Bid18Size         float64
	Bid19Price        float64
	Bid19Size         float64
	Bid20Price        float64
	Bid20Size         float64
}

// 已处理行数游标：记录每个文件已处理到的行，用于增量读取
var (
	processedRows   = make(map[string]uint64)
	processedRowsMu sync.Mutex
)

func parseH5File(filename string) ([]QuoteData, error) {
	exchange := path.Base(path.Dir(filename))
	items := strings.Split(path.Base(filename), "_")
	transaction := items[0]

	symbol := items[1]

	symbol = strings.TrimSuffix(symbol, ".h5")

	// 打开 hdf5 文件 (只读)
	file, err := hdf5.OpenFile(filename, hdf5.F_ACC_RDONLY)
	if err != nil {
		log.Fatalf("无法打开文件: %v", err)
	}
	defer file.Close()

	// 打开数据集
	dataset, err := file.OpenDataset("df")
	if err != nil {
		log.Fatalf("无法打开数据集: %v", err)
	}
	defer dataset.Close()

	// 获取数据类型和空间
	space := dataset.Space()
	dims, _, _ := space.SimpleExtentDims()
	log.Printf("数据维度: %v\n", dims)

	// 计算增量读取范围
	total := uint64(dims[0])
	processedRowsMu.Lock()
	startRow := processedRows[filename]
	if total < startRow {
		// 文件被替换/截断，重置游标
		startRow = 0
	}
	processedRowsMu.Unlock()

	if startRow == total {
		// 无新增数据
		return nil, nil
	}

	// 读取全量后内存切片，简化 HDF5 超切片处理
	data := make([]QuoteData, total)
	if err := dataset.Read(&data); err != nil {
		log.Fatalf("读取失败: %v", err)
	}

	appendData := data[startRow:total]
	if len(appendData) > 0 {
		limit := len(appendData)
		if limit > 5 {
			limit = 5
		}
		log.Printf("文件 %s 新增 %d 行，样例: %+v\n", path.Base(filename), len(appendData), appendData[:limit])
	}

	ticks := make([]*proto.Tick, 0)
	for _, quote := range appendData {
		ticks = append(ticks, &proto.Tick{
			ReceiveTime: float64(quote.ReceiveTimeStamp / int64(time.Second)),
			Symbol:      symbol,
			MarketType:  transaction,
			Exchange:    exchange,
			BestBidPx:   quote.BestBidPrice,
			BestBidSz:   quote.BestBidSize,
			BestAskPx:   quote.BestAskPrice,
			BestAskSz:   quote.BestAskSize,
		})
		if len(ticks) >= 100 {
			err = client.InsertTicks(context.Background(), ticks)
			if err != nil {
				log.Fatalf("插入失败: %v", err)
			}
			ticks = make([]*proto.Tick, 0)
		}
	}
	if len(ticks) > 0 {
		err = client.InsertTicks(context.Background(), ticks)
		if err != nil {
			log.Fatalf("插入失败: %v", err)
		}
	}

	// 更新游标
	processedRowsMu.Lock()
	processedRows[filename] = total
	processedRowsMu.Unlock()

	return appendData, nil
}

func watchFile(dir string) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Fatalf("无法创建 watcher: %v", err)
	}
	defer watcher.Close()

	// 递归添加现有子目录
	addRecursive := func(root string) error {
		return filepath.WalkDir(root, func(p string, d os.DirEntry, walkErr error) error {
			if walkErr != nil {
				log.Printf("遍历目录出错: %v", walkErr)
				return nil
			}
			if d.IsDir() {
				if strings.Contains(*exclude_dir, p) {
					log.Printf("排除目录: %s", p)
					return nil
				}
				if err := watcher.Add(p); err != nil {
					log.Printf("添加监听失败 %s: %v", p, err)
				} else {
					log.Printf("添加监听成功: %s", p)
				}
			}
			return nil
		})
	}

	if err := addRecursive(dir); err != nil {
		log.Fatalf("初始化递归监听失败: %v", err)
	}

	// 去抖动：同一文件在短时间内多次写入只触发一次处理
	debouncers := make(map[string]*time.Timer)
	var mu sync.Mutex
	debounce := func(name string, d time.Duration, fn func()) {
		mu.Lock()
		if t, ok := debouncers[name]; ok {
			t.Stop()
		}
		t := time.AfterFunc(d, fn)
		debouncers[name] = t
		mu.Unlock()
	}

	for {
		select {
		case event := <-watcher.Events:
			log.Printf("event: %v", event)
			// 新建目录：追加递归监听
			if event.Op&fsnotify.Create == fsnotify.Create {
				info, statErr := os.Stat(event.Name)
				if statErr == nil && info.IsDir() {
					if err := addRecursive(event.Name); err != nil {
						log.Printf("对子目录添加监听失败 %s: %v", event.Name, err)
					}
					continue
				}
			}
			// 文件内容变更：处理数据文件
			if event.Op&fsnotify.Write == fsnotify.Write {
				if strings.ToLower(filepath.Ext(event.Name)) != ".h5" {
					continue
				}
				name := event.Name
				// 300ms 去抖动窗口，可按需调整
				debounce(name, 1*time.Second, func() {
					// 在新 goroutine 中处理，避免阻塞事件循环
					parseH5File(name)
				})
			}
		case err := <-watcher.Errors:
			log.Fatalf("无法监听文件: %v", err)
		}
	}
}

var target = flag.String("target", "localhost:9090", "target")
var monitor_dir = flag.String("monitor_dir", "/home/zychen/A01_go/IT_exchangePlugin/pluginQuoteDataStrategy", "monitor_dir")
var exclude_dir = flag.String("exclude_dir", "", "exclude_dir")

func main() {
	flag.Parse()
	err := client.InitPool(context.Background(), *target, 10, 10, 10*time.Second, 10*time.Second)
	if err != nil {
		log.Fatalf("无法创建连接池: %v", err)
	}
	watchFile(*monitor_dir)
}
