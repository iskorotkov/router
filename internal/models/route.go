package models

import (
	"gorm.io/gorm"
)

const (
	RouteTypeRedirect RouteType = "redirect"
	RouteTypeProxy    RouteType = "proxy"
)

type RouteType string

type Route struct {
	gorm.Model
	From string
	To   string
	Type RouteType
}
