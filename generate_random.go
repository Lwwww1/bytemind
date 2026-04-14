package main

import (
	"fmt"
	"math/rand"
	"os"
	"time"
)

func main() {
	// 设置随机种子
	rand.Seed(time.Now().UnixNano())
	
	// 桌面路径
	desktopPath := "C:\\Users\\wheat\\Desktop\\随机数.txt"
	
	// 创建文件
	file, err := os.Create(desktopPath)
	if err != nil {
		fmt.Printf("创建文件失败: %v\n", err)
		return
	}
	defer file.Close()
	
	// 生成100个随机数并写入文件
	for i := 1; i <= 100; i++ {
		// 生成1到1000之间的随机整数
		randomNum := rand.Intn(1000) + 1
		
		// 写入文件
		if i == 100 {
			fmt.Fprintf(file, "%d", randomNum)
		} else {
			fmt.Fprintf(file, "%d\n", randomNum)
		}
	}
	
	fmt.Printf("成功生成100个随机数到: %s\n", desktopPath)
}