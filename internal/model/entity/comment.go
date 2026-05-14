package entity

import "gorm.io/gorm"

type Comment struct {
	ID             int64          `gorm:"primaryKey;autoIncrement:false" json:"id"`
	VideoID        int64          `gorm:"not null;index:idx_video_root_created"  json:"video_id"`
	UserID         int64          `gorm:"not null;index:idx_user_created"        json:"user_id"`
	ParentID       int64          `gorm:"not null;default:0"                     json:"parent_id"`
	RootID         int64          `gorm:"not null;default:0;index:idx_video_root_created" json:"root_id"`
	ReplyToUserID  int64          `gorm:"not null;default:0"                     json:"reply_to_user_id"`
	Content        string         `gorm:"size:1000;not null"                     json:"content"`
	LikeCount      int            `gorm:"not null;default:0"                     json:"like_count"`
	CreatedAt      int64          `gorm:"autoCreateTime:milli;index:idx_video_root_created;index:idx_user_created" json:"created_at"`
	DeletedAt      gorm.DeletedAt `gorm:"index"                                  json:"-"`

}
