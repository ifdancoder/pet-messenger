export type User = { id: string; username: string; display_name: string; avatar_url: string }

export type Session = { access_token: string; access_token_expires_at: string; refresh_token: string; user: User }

export type Conversation = {
  id: string
  kind: string
  title: string
  avatar_url?: string
  last_message_at: string
  direct_username?: string
  direct_display_name?: string
  direct_avatar_url?: string
  unread_count: number
}

export type Notification = { id: string; kind: string; payload: { conversation_id?: string }; created_at: string; read_at: string | null }

export type Message = {
  id: string
  seq?: number
  sender_id?: string
  type?: string
  content: string
  created_at: string
  read_by?: number
  recipient_count?: number
}

export type MemberProfile = {
  user_id: string
  username: string
  display_name: string
  avatar_url: string
  role: string
  admin_tier: string
  is_creator: boolean
}
