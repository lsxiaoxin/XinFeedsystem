package entity

import "gorm.io/gorm"

type Video struct {
	ID           int64          `gorm:"primaryKey;autoIncrement:false" json:"id"`
	AuthorID     int64          `gorm:"not null;index"                 json:"author_id"`
	Title        string         `gorm:"size:128;not null"              json:"title"`
	PlayURL      string         `gorm:"size:512;not null"              json:"play_url"`
	CoverURL     string         `gorm:"size:512;not null"              json:"cover_url"`
	Duration     int            `gorm:"not null;default:0"             json:"duration"`
	LikeCount    int            `gorm:"not null;default:0"             json:"like_count"`
	CommentCount int            `gorm:"not null;default:0"             json:"comment_count"`
	PlayCount    int64          `gorm:"not null;default:0"             json:"play_count"`
	Status       int8           `gorm:"not null;default:1"             json:"status"`
	CreatedAt    int64          `gorm:"autoCreateTime:milli"           json:"created_at"`
	UpdatedAt    int64          `gorm:"autoUpdateTime:milli"           json:"updated_at"`
	DeletedAt    gorm.DeletedAt `gorm:"index"                          json:"-"`
}
