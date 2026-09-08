// internal/fx/http.go
package fxapp

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"go.uber.org/fx"

	httpadapter "pet-messenger/internal/adapter/http"
	"pet-messenger/internal/config"
	"pet-messenger/internal/database"
	infraauth "pet-messenger/internal/infrastructure/auth"
)

func newGinEngine(
	db database.Service,
	authHandler *httpadapter.AuthHandler,
	convHandler *httpadapter.ConversationHandler,
	msgHandler *httpadapter.MessageHandler,
	verifier *infraauth.JWTVerifier,
) *gin.Engine {
	r := gin.Default()
	r.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"http://localhost:5173"},
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS", "PATCH"},
		AllowHeaders:     []string{"Accept", "Authorization", "Content-Type"},
		ExposeHeaders:    []string{"Location"},
		AllowCredentials: true,
	}))

	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, db.Health())
	})

	authHandler.RegisterRoutes(r.Group("/auth"))

	conversations := r.Group("/conversations")
	conversations.Use(httpadapter.AuthMiddleware(verifier))
	convHandler.RegisterRoutes(conversations)
	msgHandler.RegisterRoutes(conversations)

	return r
}

func newHTTPServer(lc fx.Lifecycle, cfg *config.Config, engine *gin.Engine) *http.Server {
	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Port),
		Handler:      engine,
		IdleTimeout:  time.Minute,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			ln, err := net.Listen("tcp", srv.Addr)
			if err != nil {
				return fmt.Errorf("listen on %s: %w", srv.Addr, err)
			}
			go func() {
				if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
					log.Printf("http server error: %v", err)
				}
			}()
			return nil
		},
		OnStop: func(ctx context.Context) error {
			return srv.Shutdown(ctx)
		},
	})

	return srv
}

var HTTPModule = fx.Module("http",
	fx.Provide(newGinEngine, newHTTPServer),
	// force the (otherwise unconsumed) *http.Server to be constructed so its
	// OnStart/OnStop lifecycle hooks actually run.
	fx.Invoke(func(*http.Server) {}),
)
