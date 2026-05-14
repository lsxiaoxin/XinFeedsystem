package entity

import "gorm.io/gorm"

type VideoLike struct {
	ID        int64          `gorm:"primaryKey;autoIncrement:false" json:"id"`
	UserID    int64          `gorm:"not null;uniqueIndex:uk_user_video" json:"user_id"`
	VideoID   int64          `gorm:"not null;uniqueIndex:uk_user_video;index:idx_video" json:"video_id"`
	CreatedAt int64          `gorm:"autoCreateTime:milli;index:idx_video" json:"created_at"`
	DeletedAt gorm.DeletedAt `gorm:"index"                              json:"-"`
}
