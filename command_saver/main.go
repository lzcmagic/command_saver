package main

import (
	"database/sql"
	"fmt"
	"os"
	"strings"
	"time"
	_ "github.com/mattn/go-sqlite3"
	"os/exec"
)

func initDB() *sql.DB {
	// 获取用户主目录
	home := os.Getenv("HOME")
	dbDir := home + "/.command_saver"
	dbPath := dbDir + "/commands.db"
	
	// 检查目录是否存在，如果不存在则创建
	if _, err := os.Stat(dbDir); os.IsNotExist(err) {
		fmt.Printf("数据库目录不存在，正在创建: %s\n", dbDir)
		if err := os.MkdirAll(dbDir, 0755); err != nil {
			panic(fmt.Sprintf("创建数据库目录失败: %v", err))
		}
	}
	
	// 打开或创建数据库
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		panic(fmt.Sprintf("打开数据库失败: %v", err))
	}

	// 创建表（如果不存在）
	createTableSQL := `
	PRAGMA foreign_keys = ON;
	PRAGMA encoding = 'UTF-8';
	
	CREATE TABLE IF NOT EXISTS command_history (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		command TEXT NOT NULL COLLATE NOCASE,
		description TEXT COLLATE NOCASE,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);`

	if _, err = db.Exec(createTableSQL); err != nil {
		panic(fmt.Sprintf("创建数据表失败: %v", err))
	}

	return db
}

func saveCommand(db *sql.DB, command, description string) {
	stmt, err := db.Prepare("INSERT INTO command_history(command, description, created_at) VALUES(?, ?, ?)")
	if err != nil {
		fmt.Println("准备SQL语句时出错:", err)
		return
	}
	defer stmt.Close()

	_, err = stmt.Exec(command, description, time.Now())
	if err != nil {
		fmt.Println("保存命令时出错:", err)
		return
	}

	fmt.Printf("命令已保存：%s\n", command)
}

func getLastCommand() string {
	// 检测当前的 shell
	shell := os.Getenv("SHELL")

	var histFile string
	var isZsh bool

	// 判断shell类型
	if strings.Contains(shell, "zsh") {
		histFile = os.Getenv("HOME") + "/.zsh_history"
		isZsh = true
	} else if strings.Contains(shell, "bash") {
		histFile = os.Getenv("HOME") + "/.bash_history"
		isZsh = false
	} else {
		fmt.Println("不支持的shell类型")
		return ""
	}

	// 检查历史文件是否存在
	if _, err := os.Stat(histFile); os.IsNotExist(err) {
		fmt.Printf("历史文件不存在: %s\n", histFile)
		return ""
	}

	// 读取最后几行命令
	cmd := exec.Command("tail", "-n", "2", histFile)
	output, err := cmd.Output()
	if err != nil {
		fmt.Printf("读取历史文件出错: %v\n", err)
		return ""
	}

	// 按行分割输出
	lines := strings.Split(string(output), "\n")
	
	// 获取最后一行非空命令
	var lastCmd string
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}

		// 处理 zsh 特殊格式
		if isZsh && strings.Contains(line, ";") {
			parts := strings.SplitN(line, ";", 2)
			if len(parts) >= 2 {
				line = strings.TrimSpace(parts[1])
			}
		}

		// 排除当前程序的命令和空命令
		if line == "" || 
		   strings.HasPrefix(line, "./cs") || 
		   strings.HasPrefix(line, "cs ") || 
		   strings.HasPrefix(line, "tail") {
			continue
		}

		lastCmd = line
		break
	}

	if lastCmd == "" {
		return ""
	}

	// 检查命令是否存在且可执行
	cmdParts := strings.Fields(lastCmd)
	if len(cmdParts) == 0 {
		fmt.Println("无效的命令格式")
		return ""
	}

	// 使用which命令检查命令是否存在
	checkCmd := exec.Command("which", cmdParts[0])
	if err := checkCmd.Run(); err != nil {
		fmt.Println("上一条命令执行出错，不进行保存")
		return ""
	}

	return lastCmd
}

func listCommands(db *sql.DB) {
	rows, err := db.Query(`
		SELECT id, command, description, created_at 
		FROM command_history 
		ORDER BY created_at DESC
	`)
	if err != nil {
		fmt.Println("查询数据库时出错:", err)
		return
	}
	defer rows.Close()

	fmt.Println("\n保存的命令历史:")
	fmt.Println("--------------------------------------------------------------------------------")
	fmt.Printf("%-6s | %-30s | %-30s | %s\n", "ID", "时间", "命令", "描述")
	fmt.Println("--------------------------------------------------------------------------------")

	for rows.Next() {
		var id int
		var command, description string
		var createdAt time.Time
		err := rows.Scan(&id, &command, &description, &createdAt)
		if err != nil {
			fmt.Println("读取数据时出错:", err)
			continue
		}
		timeStr := createdAt.Format("2006-01-02 15:04:05")
		if description == "" {
			description = "-"
		}
		fmt.Printf("%-6d | %-30s | %-30s | %s\n", id, timeStr, command, description)
	}
	fmt.Println("--------------------------------------------------------------------------------")
}

// 添加新的函数来显示按天分组的命令
func listCommandsByDay(db *sql.DB) {
	// 查询近7天的命令，按天分组
	rows, err := db.Query(`
		SELECT 
			DATE(created_at) as day,
			GROUP_CONCAT(id) as ids,
			GROUP_CONCAT(command) as commands,
			GROUP_CONCAT(description) as descriptions,
			GROUP_CONCAT(created_at) as times
		FROM command_history 
		WHERE created_at >= date('now', '-7 days')
		GROUP BY DATE(created_at)
		ORDER BY day DESC
	`)
	if err != nil {
		fmt.Println("查询数据库时出错:", err)
		return
	}
	defer rows.Close()

	fmt.Println("\n最近7天的命令历史:")
	
	for rows.Next() {
		var day string
		var ids, commands, descriptions, times string
		
		err := rows.Scan(&day, &ids, &commands, &descriptions, &times)
		if err != nil {
			fmt.Println("读取数据时出错:", err)
			continue
		}
		
		// 打印日期分隔线
		fmt.Printf("\n=== %s ===\n", day)
		fmt.Println("--------------------------------------------------------------------------------")
		fmt.Printf("%-6s | %-30s | %-30s | %s\n", "ID", "时间", "命令", "描述")
		fmt.Println("--------------------------------------------------------------------------------")
		
		// 分割每一天的数据
		idList := strings.Split(ids, ",")
		cmdList := strings.Split(commands, ",")
		descList := strings.Split(descriptions, ",")
		timeList := strings.Split(times, ",")
		
		// 确保所有切片长度一致
		length := len(idList)
		for i := 0; i < length; i++ {
			id := idList[i]
			cmd := cmdList[i]
			desc := descList[i]
			if desc == "" {
				desc = "-"
			}
			
			// 解析并格式化时间
			t, _ := time.Parse("2006-01-02 15:04:05", strings.Split(timeList[i], ".")[0])
			timeStr := t.Format("15:04:05")
			
			fmt.Printf("%-6s | %-30s | %-30s | %s\n", id, timeStr, cmd, desc)
		}
		fmt.Println("--------------------------------------------------------------------------------")
	}
}

func cleanDatabase() {
	// 获取数据库文件路径
	home := os.Getenv("HOME")
	dbPath := home + "/.command_saver/commands.db"
	
	// 检查文件是否存在
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		fmt.Println("数据库文件不存在")
		return
	}
	
	// 询问用户确认
	fmt.Print("警告：此操作将删除所有保存的命令历史记录，确定要继续吗？(y/N): ")
	var response string
	fmt.Scanln(&response)
	
	// 检查用户响应
	response = strings.ToLower(strings.TrimSpace(response))
	if response != "y" && response != "yes" {
		fmt.Println("操作已取消")
		return
	}
	
	// 删除数据库文件
	err := os.Remove(dbPath)
	if err != nil {
		fmt.Printf("删除数据库文件失败: %v\n", err)
		return
	}
	
	fmt.Println("数据库已清除")
}

func showHelp() {
	fmt.Println("使用方法: ")
	fmt.Println("  cs [命令说明]  - 保存上一条命令，不带说明则默认为'default'")
	fmt.Println("  cs list      - 显示所有保存的命令")
	fmt.Println("  cs -d list   - 按天显示最近7天的命令")
	fmt.Println("  cs clean     - 清除所有保存的命令")
	fmt.Println("  cs --help    - 显示帮助信息")
}

func main() {
	// 处理特殊命���
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "--help":
			showHelp()
			return
		case "list":
			db := initDB()
			defer db.Close()
			listCommands(db)
			return
		case "-d":
			if len(os.Args) > 2 && os.Args[2] == "list" {
				db := initDB()
				defer db.Close()
				listCommandsByDay(db)
				return
			}
		case "clean":
			cleanDatabase()
			return
		}
	}

	// 初始化数据库
	db := initDB()
	defer db.Close()

	// 获取上一条命令
	lastCommand := getLastCommand()
	if lastCommand == "" {
		// 如果已经打印了具体错误信息，就不再显示通用错误
		return
	}

	// 获取可选的描述
	var description string
	if len(os.Args) > 1 {
		description = strings.Join(os.Args[1:], " ")
	} else {
		description = "default"
	}

	// 保存到数据库
	saveCommand(db, lastCommand, description)
} 