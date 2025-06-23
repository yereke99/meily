package main

import (
	_ "github.com/mattn/go-sqlite3"
)

/*
func main() {
	zapLogger, err := logger.NewLogger()
	if err != nil {
		panic(err)
	}

	cfg, err := config.NewConfig()
	if err != nil {
		zapLogger.Error("error initializing config", zap.Error(err))
		return
	}

	db, err := sql.Open("sqlite3", cfg.DBName)
	if err != nil {
		zapLogger.Error("error in connect to database", zap.Error(err))
		return
	}
	defer db.Close()

	r := repository.NewUserRepository(db)
	ctx := context.Background()
	if err := r.Delete(ctx, 10); err != nil {
		zapLogger.Error("error in delete user", zap.Error(err))
	}
	zapLogger.Info("success")
}

*/
