package routes

import (
	"net/http"

	"amerimon/internal/config"
	"amerimon/internal/controllers"

	"github.com/labstack/echo/v4"
)

// Prefix es la raíz de la API REST; el proxy de Apache y el de ng serve apuntan aquí.
const Prefix = "/mon"

// Setup registra las rutas REST, el WebSocket de listen-live y el health check.
func Setup(e *echo.Echo, mgr *config.AudioStreamManager) {
	publica := e.Group(Prefix)
	privada := e.Group(Prefix)
	jwt := controllers.JWTMiddleware
	admin := controllers.AdminMiddleware

	publica.GET("/health", func(c echo.Context) error {
		status := http.StatusOK
		body := map[string]string{"status": "ok"}
		if config.App.DB != nil {
			if err := config.App.DB.PingContext(c.Request().Context()); err != nil {
				status = http.StatusServiceUnavailable
				body["status"] = "db unavailable"
			}
		}
		return c.JSON(status, body)
	})

	publica.POST("/auth/login", controllers.AdminLogin)
	publica.GET("/auth/status", controllers.AdminStatus)
	publica.POST("/auth/setup", controllers.SetupAdminPassword)
	publica.GET("/site-settings", controllers.GetSiteSettings)
	publica.GET("/weather", controllers.GetWeather)
	privada.POST("/auth/password", controllers.ChangeAdminPassword, jwt, admin)
	privada.PUT("/site-settings", controllers.SaveSiteSettings, jwt, admin)
	privada.GET("/system-info", controllers.GetSystemInfo, jwt, admin)
	privada.POST("/system/control", controllers.ControlSystem, jwt, admin)

	publica.GET("/astdb", controllers.GetAstDB)
	publica.GET("/sm_lsnodes", controllers.HandleAllStarNodes)
	publica.GET("/enlaces-externos", controllers.ListEnlacesExternos)

	privada.POST("/asterisk", controllers.SendAsteriskCmd, jwt, admin)
	privada.GET("/asterisk/commands", controllers.ListSupermonCommands, jwt, admin)
	privada.POST("/asterisk/commands", controllers.ExecuteSupermonCommand, jwt, admin)
	privada.GET("/asterisk/logs", controllers.GetAsteriskOperationLogs, jwt, admin)
	privada.POST("/asterisk/connections", controllers.AddNodeConnection, jwt, admin)

	// Rutas sin rol A: públicas; el token es opcional y, si es válido, identifica al usuario.
	opt := controllers.OptionalJWTMiddleware
	publica.GET("/allstarlink/nodes", controllers.GetAllStarLinkNodes, opt)
	publica.GET("/allstarlink/activity", controllers.GetAllStarLinkNodeActivity, opt)
	privada.GET("/allstarlink/connections/history", controllers.GetAsteriskConnectionHistory, jwt, admin)
	privada.GET("/geoip/:ip", controllers.GeoIP, jwt, admin)
	privada.GET("/allmon/config", controllers.GetAllmonConfig, jwt, admin)
	privada.PUT("/allmon/config", controllers.SaveAllmonConfig, jwt, admin)
	privada.GET("/admin/enlaces-externos", controllers.ListEnlacesExternosAdmin, jwt, admin)
	privada.POST("/admin/enlaces-externos", controllers.CreateEnlaceExterno, jwt, admin)
	privada.PUT("/admin/enlaces-externos/:id", controllers.UpdateEnlaceExterno, jwt, admin)
	privada.DELETE("/admin/enlaces-externos/:id", controllers.DeleteEnlaceExterno, jwt, admin)
	privada.GET("/private-nodes", controllers.GetPrivateNodesConfig, jwt, admin)
	privada.PUT("/private-nodes", controllers.SavePrivateNodesConfig, jwt, admin)
	privada.GET("/primary-node", controllers.GetPrimaryNodeConfig, jwt, admin)
	privada.PUT("/primary-node", controllers.SavePrimaryNodeConfig, jwt, admin)
	privada.GET("/primary-node/ami", controllers.GetPrimaryNodeAMIConfig, jwt, admin)
	privada.PUT("/primary-node/ami", controllers.SavePrimaryNodeAMIConfig, jwt, admin)
	privada.GET("/primary-node/access", controllers.GetPrimaryNodeAccess, jwt, admin)
	privada.PUT("/primary-node/access", controllers.SavePrimaryNodeAccess, jwt, admin)
	privada.DELETE("/primary-node/access", controllers.DeletePrimaryNodeAccess, jwt, admin)
	privada.POST("/primary-node/access/clear", controllers.ClearPrimaryNodeAccess, jwt, admin)
	privada.GET("/primary-node/access/export", controllers.ExportPrimaryNodeAccess, jwt, admin)
	privada.POST("/primary-node/access/import", controllers.ImportPrimaryNodeAccess, jwt, admin)

	privada.GET("/favoritos", controllers.ListFavoritos, jwt, admin)
	privada.GET("/favoritos/astdb-search", controllers.BuscarAstdbFavorito, jwt, admin)
	privada.GET("/favoritos/astdb/:numero", controllers.AstdbFavorito, jwt, admin)
	privada.POST("/favoritos", controllers.CreateFavorito, jwt, admin)
	privada.PUT("/favoritos/:id", controllers.UpdateFavorito, jwt, admin)
	privada.DELETE("/favoritos/:id", controllers.DeleteFavorito, jwt, admin)

	// ws://<host>/mon/listen-live/ws?uuid=<uuid>, tras POST /mon/listen-live/start.
	publica.GET("/listen-live/ws", config.HandleLiveListenWS(mgr), opt)
	publica.GET("/listen-live/status", controllers.ListenLiveAsteriskStatus, opt)
	publica.POST("/listen-live/start", controllers.StartListenLive, opt)
	publica.POST("/listen-live/stop", controllers.StopListenLive(mgr), opt)
	publica.GET("/listen-live/sessions", controllers.ListListenLiveSessions, opt)
	publica.POST("/listen-live/sessions", controllers.CreateListenLiveSession, opt)
	publica.GET("/listen-live/sessions/:id", controllers.GetListenLiveSession, opt)
	publica.PATCH("/listen-live/sessions/:id", controllers.UpdateListenLiveSession(mgr), opt)
	publica.DELETE("/listen-live/sessions/:id", controllers.DeleteListenLiveSession, opt)
}
