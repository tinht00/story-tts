import type {
  AppState,
  ChapterContent,
  ImportFolderRequest,
  RealtimeChapterPayload,
  RealtimeSession,
  RealtimeVoice,
  ReaderProgress,
  Story,
  StoryDetail
} from '../types'

async function request<T>(url: string, init?: RequestInit): Promise<T> {
  const response = await fetch(url, {
    headers: { 'Content-Type': 'application/json' },
    ...init
  })

  if (!response.ok) {
    const payload = await response.json().catch(() => ({}))
    throw new Error(payload.error || `HTTP ${response.status}`)
  }
  return response.json() as Promise<T>
}

export const api = {
  state: () => request<AppState>('/api/state'),
  importFolder: (payload: ImportFolderRequest) =>
    request<{ stories: Story[] }>('/api/library/import-folder', {
      method: 'POST',
      body: JSON.stringify(payload)
    }),
  stories: () => request<{ items: Story[] }>('/api/library/stories'),
  story: (id: number) => request<StoryDetail>(`/api/library/stories/${id}`),
  chapterContent: (id: number) => request<ChapterContent>(`/api/library/chapters/${id}/content`),
  progress: (storyId: number) => request<ReaderProgress>(`/api/reader/progress/${storyId}`),
  saveProgress: (payload: ReaderProgress) =>
    request<ReaderProgress>('/api/reader/progress', {
      method: 'POST',
      body: JSON.stringify(payload)
    }),
  toggleEdgeReadAloud: () =>
    request<{ status: string }>('/api/reader/edge-read-aloud/toggle', {
      method: 'POST'
    }),
  realtimeVoices: (baseUrl: string) => request<{ items: RealtimeVoice[]; defaultVoice: string }>(`${baseUrl}/voices`),
  createRealtimeSession: (
    baseUrl: string,
    payload: {
      storyId: number
      chapterId: number
      chapters: RealtimeChapterPayload[]
      voice: string
      speed: number
      pitch: number
      autoNext: boolean
      startSegmentIndex?: number
    }
  ) =>
    request<RealtimeSession>(`${baseUrl}/sessions`, {
      method: 'POST',
      body: JSON.stringify(payload)
    }),
  stopRealtimeSession: (baseUrl: string, id: string) =>
    request<{ status: string; id: string }>(`${baseUrl}/sessions/${id}/stop`, {
      method: 'POST'
    }),
  updateRealtimeControls: (
    baseUrl: string,
    id: string,
    payload: {
      voice?: string
      speed?: number
      pitch?: number
      autoNext?: boolean
    }
  ) =>
    request<RealtimeSession>(`${baseUrl}/sessions/${id}/controls`, {
      method: 'POST',
      body: JSON.stringify(payload)
    }),
  skipRealtimeNext: (baseUrl: string, id: string) =>
    request<{ status: string; id: string }>(`${baseUrl}/sessions/${id}/skip-next`, {
      method: 'POST'
    }),
  skipRealtimePrev: (baseUrl: string, id: string) =>
    request<{ status: string; id: string }>(`${baseUrl}/sessions/${id}/skip-prev`, {
      method: 'POST'
    }),
  seekRealtimeSession: (
    baseUrl: string,
    id: string,
    payload: {
      chapterId: number
      segmentIndex: number
    }
  ) =>
    request<RealtimeSession>(`${baseUrl}/sessions/${id}/seek`, {
      method: 'POST',
      body: JSON.stringify(payload)
    })
}
