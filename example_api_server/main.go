package main

import (
	"archive/zip"
	"database/sql"
	"fmt"
	_ "github.com/go-sql-driver/mysql"
	"github.com/jamf/go-mysqldump"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
	"k8s.io/apimachinery/pkg/util/json"
	"log"
	"net/http"
	"os"
	"path"
	"time"
)

type Config struct {
	DB string `yaml:"db"`
}

func backupCommand(configFile string) *cobra.Command {
	var cfg Config
	var dir = "/tmp/"
	cmd := &cobra.Command{ //  备份数据库
		Use:   "backup",
		Short: "Example Api Server DB Backup",
		RunE: func(cmd *cobra.Command, args []string) error {
			b, err := os.ReadFile(configFile)
			if err != nil {
				return err
			}
			if err = yaml.Unmarshal(b, &cfg); err != nil {
				return err
			}
			db, err := sql.Open("mysql", cfg.DB)
			if err != nil {
				return errors.Wrap(err, "failed to connect to database")
			}
			defer db.Close()
			fileName := path.Join(dir, fmt.Sprintf("%s.sql.zip", time.Now().Format("20060102_150405")))
			out, err := os.Create(fileName)
			if err != nil {
				return err
			}
			defer out.Close()
			zipWriter := zip.NewWriter(out)
			defer zipWriter.Close()
			w, err := zipWriter.Create("db.sql")
			if err != nil {
				return err
			}
			dumper := &mysqldump.Data{Out: w, Connection: db, LockTables: false}
			if err = dumper.Dump(); err != nil {
				os.Remove(fileName)
				return err
			}
			log.Printf("backup file: %s\n", fileName)
			return nil
		},
	}
	cmd.Flags().StringVar(&dir, "dir", dir, "backup dir")
	return cmd
}
func main() {
	var configFile = "cfg.yaml"
	cmd := &cobra.Command{
		Use:                   "server",
		Short:                 "Example Api Server",
		SilenceUsage:          true,
		SilenceErrors:         true,
		DisableFlagsInUseLine: true,
		TraverseChildren:      true,
		RunE: func(cmd *cobra.Command, args []string) error {
			var cfg Config
			b, err := os.ReadFile(configFile)
			if err != nil {
				return err
			}
			if err = yaml.Unmarshal(b, &cfg); err != nil {
				return err
			}
			return serve(cfg)
		},
	}
	cmd.AddCommand(backupCommand(configFile))
	cmd.Flags().StringVarP(&configFile, "cfg", "c", configFile, `cfg file`)
	if err := cmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

type user struct {
	Username string `json:"username"`
}

func serve(cfg Config) error {
	db, err := sql.Open("mysql", cfg.DB)
	if err != nil {
		return err
	}
	defer db.Close()
	// 健康检测
	http.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})
	// 测试接口
	http.HandleFunc("/api/v1/users", func(w http.ResponseWriter, r *http.Request) {
		var users []user
		rows, err := db.Query("SELECT username FROM operator_example_user")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer rows.Close()
		for rows.Next() {
			var u user
			if err = rows.Scan(&u.Username); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			users = append(users, u)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(users)
	})
	log.Printf("Listening on port 8111")
	return http.ListenAndServe(":8111", nil)
}
