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
	wsadapter "pet-messenger/internal/adapter/ws"
	authweb "pet-messenger/internal/auth/adapter/in/web"
	"pet-messenger/internal/config"
	"pet-messenger/internal/conversation/adapter/in/web"
	"pet-messenger/internal/database"
	e2eeweb "pet-messenger/internal/e2ee/adapter/in/web"
	infraauth "pet-messenger/internal/infrastructure/auth"
	messageweb "pet-messenger/internal/message/adapter/in/web"
	notificationweb "pet-messenger/internal/notification/adapter/in/web"
	pushtokenweb "pet-messenger/internal/pushtoken/adapter/in/web"
	userweb "pet-messenger/internal/user/adapter/in/web"
)

func newGinEngine(
	cfg *config.Config,
	db database.Service,
	authHandler *authweb.AuthHandler,
	userHandler *userweb.UserHandler,
	e2eeDeviceHandler *e2eeweb.E2EEDeviceHandler,
	convHandler *web.ConversationHandler,
	msgHandler *messageweb.MessageHandler,
	notifHandler *notificationweb.NotificationHandler,
	pushTokenHandler *pushtokenweb.PushTokenHandler,
	wsHandler *wsadapter.WSHandler,
	callHandler *httpadapter.CallHandler,
	verifier *infraauth.JWTVerifier,
) *gin.Engine {
	r := gin.Default()
	r.Use(cors.New(cors.Config{
		AllowOrigins:     cfg.CORS.AllowedOrigins,
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS", "PATCH"},
		AllowHeaders:     []string{"Accept", "Authorization", "Content-Type"},
		ExposeHeaders:    []string{"Location"},
		AllowCredentials: true,
	}))

	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, db.Health())
	})

	authHandler.RegisterRoutes(r.Group("/auth"))
	users := r.Group("/users")
	users.Use(httpadapter.AuthMiddleware(verifier))
	userHandler.RegisterRoutes(users)
	e2eeDeviceHandler.RegisterRoutes(users)

	conversations := r.Group("/conversations")
	conversations.Use(httpadapter.AuthMiddleware(verifier))
	convHandler.RegisterRoutes(conversations)
	msgHandler.RegisterRoutes(conversations)
	callHandler.RegisterConversationRoutes(conversations)
	e2eeDeviceHandler.RegisterConversationRoutes(conversations)

	calls := r.Group("/calls")
	calls.Use(httpadapter.AuthMiddleware(verifier))
	callHandler.RegisterRoutes(calls)
	e2eeDeviceHandler.RegisterCallRoutes(calls)

	notifs := r.Group("/notifications")
	notifs.Use(httpadapter.AuthMiddleware(verifier))
	notifHandler.RegisterRoutes(notifs)

	pushToks := r.Group("/push-tokens")
	pushToks.Use(httpadapter.AuthMiddleware(verifier))
	pushTokenHandler.RegisterRoutes(pushToks)

	wsHandler.RegisterRoutes(r)

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
