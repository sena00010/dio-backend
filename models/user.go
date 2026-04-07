package models

import "time"

type User struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	FirebaseUID  *string   `gorm:"size:128;uniqueIndex" json:"firebase_uid,omitempty"`
	FirstName    string    `gorm:"size:100;not null" json:"first_name"`
	LastName     string    `gorm:"size:100;not null" json:"last_name"`
	Email        string    `gorm:"size:255;uniqueIndex;not null" json:"email"`
	PhoneNumber  string    `gorm:"size:20" json:"phone_number,omitempty"`
	PasswordHash string    `gorm:"size:255" json:"-"`
	AuthProvider string    `gorm:"size:20;not null;default:'email'" json:"auth_provider"`
	Company      Company   `gorm:"foreignKey:UserID" json:"company,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}
