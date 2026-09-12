export type User = { id: string; username: string; display_name: string }

export type Session = { access_token: string; user: User }

export type Conversation = {
  id: string
  kind: string
  title: string
  last_message_at: string
}

export type Message = {
  id: string
  seq?: number
  sender_id?: string
  content: string
  created_at: string
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
