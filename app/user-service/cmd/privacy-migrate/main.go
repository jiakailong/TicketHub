package main

import (
	"context"
	"flag"
	"fmt"
	"log"

	usermysql "tickethub/app/user-service/internal/infrastructure/mysql"
	"tickethub/pkg/config"
	"tickethub/pkg/db"
	"tickethub/pkg/privacy"
)

func main() {
	conf := flag.String("conf", "app/user-service/configs/config.yaml", "config file path")
	flag.Parse()
	cfg, err := config.Load(*conf)
	if err != nil {
		log.Fatal(err)
	}
	protector, err := privacy.NewProtector(cfg.Privacy.ActiveKeyVersion, cfg.Privacy.EncryptionKeys, cfg.Privacy.LookupKey)
	if err != nil {
		log.Fatal(err)
	}
	database, err := db.OpenMySQL(context.Background(), cfg.MySQL)
	if err != nil {
		log.Fatal(err)
	}
	defer database.Close()
	result, err := usermysql.NewLegacyPrivacyMigrator(database, protector).Run(context.Background())
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("privacy migration complete: users=%d ticket_users=%d\n", result.Users, result.TicketUsers)
}
