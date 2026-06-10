package main

import (
	"context"
	"fmt"
	"os"

	"irollhub/handler"
	"irollhub/middleware"
	"irollhub/store"

	"github.com/gin-gonic/gin"
)

func main() {
	cfgPath := "config.yaml"
	if len(os.Args) > 1 {
		cfgPath = os.Args[1]
	}

	cfg, err := LoadConfig(cfgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "config error: %v\n", err)
		os.Exit(1)
	}

	db, err := store.OpenDB(cfg.DB)
	if err != nil {
		fmt.Fprintf(os.Stderr, "db error: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	mc, err := store.NewMinIOClient(cfg.MinIO)
	if err != nil {
		fmt.Fprintf(os.Stderr, "minio error: %v\n", err)
		os.Exit(1)
	}
	if err := mc.EnsureBucket(context.Background()); err != nil {
		fmt.Fprintf(os.Stderr, "bucket error: %v\n", err)
		os.Exit(1)
	}

	gin.SetMode(gin.ReleaseMode)
	r := gin.Default()

	// Handlers
	authH := handler.NewAuthHandler(
		db,
		cfg.OAuth.GithubClientID,
		cfg.OAuth.GithubClientSecret,
		cfg.OAuth.GoogleClientID,
		cfg.OAuth.GoogleClientSecret,
		cfg.OAuth.RedirectBase,
	)
	orgH := handler.NewOrgHandler(db)
	pkgH := handler.NewPackageHandler(db)
	verH := handler.NewVersionHandler(db, mc)
	searchH := handler.NewSearchHandler(db)

	// Routes
	api := r.Group("/api/v1")
	{
		auth := api.Group("/auth")
		{
			auth.GET("/github", authH.GithubStart)
			auth.GET("/github/callback", authH.GithubCallback)
			auth.GET("/google", authH.GoogleStart)
			auth.GET("/google/callback", authH.GoogleCallback)
			auth.GET("/me", middleware.AuthRequired(db), authH.Me)
			auth.POST("/keys", middleware.AuthRequired(db), authH.CreateKey)
			auth.DELETE("/keys/:key_id", middleware.AuthRequired(db), authH.RevokeKey)
		}

		orgs := api.Group("/orgs")
		{
			orgs.GET("", orgH.List)
			orgs.POST("", middleware.AuthRequired(db), orgH.Create)
			orgs.GET("/:org", orgH.Get)
		}

		pkgs := orgs.Group("/:org/packages")
		{
			pkgs.GET("", pkgH.List)
			pkgs.POST("", middleware.AuthRequired(db), pkgH.Create)
			pkgs.GET("/:pkg", pkgH.Get)
			pkgs.DELETE("/:pkg", middleware.AuthRequired(db), pkgH.Delete)
		}

		vers := pkgs.Group("/:pkg/versions")
		{
			vers.GET("", verH.List)
			vers.POST("", middleware.AuthRequired(db), verH.Upload)
			vers.GET("/:ver", verH.Get)
			vers.GET("/:ver/download", verH.Download)
			vers.DELETE("/:ver", middleware.AuthRequired(db), verH.Delete)
		}

		api.GET("/search", searchH.Search)
	}

	fmt.Printf("irollhub starting on %s\n", cfg.Listen)
	if err := r.Run(cfg.Listen); err != nil {
		fmt.Fprintf(os.Stderr, "server error: %v\n", err)
		os.Exit(1)
	}
}
