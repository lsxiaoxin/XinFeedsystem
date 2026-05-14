package entity

import "gorm.io/gorm"

type Follow struct {
	ID         int64          `gorm:"primaryKey;autoIncrement:false"             json:"id"`
	FollowerID int64          `gorm:"not null;uniqueIndex:uk_follower_followee"  json:"follower_id"`
	FolloweeID int64          `gorm:"not null;uniqueIndex:uk_follower_followee;index:idx_followee_follower" json:"followee_id"`
	CreatedAt  int64          `gorm:"autoCreateTime:milli"                       json:"created_at"`
	DeletedAt  gorm.DeletedAt `gorm:"index"                                      json:"-"`
}
