package entity

import "gorm.io/gorm"

type User struct {
	ID            int64          `gorm:"primaryKey;autoIncrement:false" json:"id"`
	Username      string         `gorm:"uniqueIndex;size:32;not null"   json:"username"`
	PasswordHash  string         `gorm:"size:72;not null"               json:"-"`
	Nickname      string         `gorm:"size:32;not null"               json:"nickname"`
	Avatar        string         `gorm:"size:255"                       json:"avatar"`
	Signature     string         `gorm:"size:140"                       json:"signature"`
	FollowCount   int            `gorm:"not null;default:0"             json:"follow_count"`
	FollowerCount int            `gorm:"not null;default:0"             json:"follower_count"`
	CreatedAt     int64          `gorm:"autoCreateTime:milli"           json:"created_at"`
	UpdatedAt     int64          `gorm:"autoUpdateTime:milli"           json:"updated_at"`
	DeletedAt     gorm.DeletedAt `gorm:"index"                          json:"-"`
}
