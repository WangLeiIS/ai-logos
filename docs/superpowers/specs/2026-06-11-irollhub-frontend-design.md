# irollhub Frontend Design Spec

**Date:** 2026-06-11  
**Status:** Approved  
**Scope:** MVP - 只读功能优先

## Overview

为 irollhub 创建一个简约现代风格的 React 前端，提供浏览、搜索和查看 .iroll 包的功能。前端独立部署，通过 CORS 调用 irollhub API。

## Target Audience

公开用户，类似 Docker Hub 的体验。

## Technology Stack

- **框架:** Vite + React + TypeScript
- **路由:** React Router v6
- **样式:** TailwindCSS
- **部署:** 独立部署（静态文件托管）

**选择理由:** 轻量、现代工具链、开发体验好，适合只读功能的 SPA。

## Page Structure & Routing

```
/                    - 首页：搜索栏 + 热门包展示
/search              - 搜索结果页
/orgs                - 组织列表
/orgs/:org           - 组织详情页（该组织的所有包）
/orgs/:org/packages/:pkg  - 包详情页（包含版本列表）
```

## Component Architecture

```
src/
├── components/
│   ├── layout/
│   │   ├── Header.tsx          # 顶部导航：Logo + 搜索框 + 链接
│   │   └── Footer.tsx          # 页脚：版权信息
│   ├── home/
│   │   └── Hero.tsx            # 首页搜索区
│   ├── package/
│   │   ├── PackageCard.tsx     # 包卡片
│   │   ├── PackageList.tsx     # 包列表
│   │   ├── PackageDetail.tsx   # 包详情
│   │   └── VersionList.tsx     # 版本列表
│   ├── org/
│   │   ├── OrgCard.tsx         # 组织卡片
│   │   └── OrgList.tsx         # 组织列表
│   └── search/
│       └── SearchBar.tsx       # 搜索组件
├── pages/
│   ├── HomePage.tsx
│   ├── SearchPage.tsx
│   ├── OrgsPage.tsx
│   ├── OrgDetailPage.tsx
│   └── PackageDetailPage.tsx
├── api/
│   └── client.ts               # API 客户端
├── types/
│   └── index.ts                # TypeScript 类型定义
├── App.tsx
└── main.tsx
```

## Data Layer

### API Client

```typescript
// api/client.ts
const API_BASE_URL = import.meta.env.VITE_API_BASE_URL || 'http://localhost:8080'

export const api = {
  search: (q: string) => fetch(`${API_BASE_URL}/api/v1/search?q=${q}`),
  listOrgs: (limit = 20, offset = 0) => 
    fetch(`${API_BASE_URL}/api/v1/orgs?limit=${limit}&offset=${offset}`),
  getOrg: (org: string) => 
    fetch(`${API_BASE_URL}/api/v1/orgs/${org}`),
  getPackages: (org: string) => 
    fetch(`${API_BASE_URL}/api/v1/orgs/${org}/packages`),
  getPackage: (org: string, pkg: string) => 
    fetch(`${API_BASE_URL}/api/v1/orgs/${org}/packages/${pkg}`),
}
```

### TypeScript Types

```typescript
export interface Organization {
  id: number;
  name: string;
  avatar_url: string;
  created_at: string;
}

export interface Package {
  id: number;
  name: string;
  description: string;
  tags: string;
  downloads: number;
  created_at: string;
  updated_at: string;
}

export interface Version {
  id: number;
  version: string;
  object_key: string;
  file_size: number;
  checksum: string;
  created_at: string;
}

export interface SearchResponse {
  data: Array<{
    org: Organization;
    package: Package;
  }>;
}
```

## UI Design

### Color Scheme

- 白底为主，浅灰辅助
- 黑色强调，无多余色彩
- 边框使用浅灰色

### TailwindCSS Usage

- `bg-white` - 主背景
- `text-gray-900` - 主文字
- `text-gray-600` - 次要文字
- `border-gray-200` - 边框
- `hover:bg-gray-50` - 轻微交互反馈
- `transition-all` - 平滑过渡

### Animations

- 卡片 hover 轻微上移（4px）
- 页面切换淡入淡出（200ms）
- 按钮点击反馈（100ms）

## Layout Patterns

**首页:** 大搜索框居中 + 下方热门包卡片网格

**列表页:** 简洁的卡片/表格布局

**详情页:** 左侧信息，右侧版本列表

## Deployment

### Build Commands

```bash
npm run dev      # 开发
npm run build    # 构建
npm run preview  # 预览构建结果
```

### Environment Variables

```bash
# .env
VITE_API_BASE_URL=http://localhost:8080
```

### Output

纯静态文件在 `dist/` 目录，可部署到任何静态托管服务。

### CORS Configuration

irollhub 侧需要添加 CORS 支持：

```go
c.Writer.Header().Set("Access-Control-Allow-Origin", "https://your-frontend.com")
c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type")
```

## MVP Scope

**包含:**
- 首页搜索与热门包展示
- 组织列表与详情
- 包列表与详情
- 版本列表查看
- 搜索功能

**不包含:**
- 用户登录/OAuth
- 上传/创建/删除等写操作
- 下载统计等高级功能

## Future Considerations

- 如果需要 SEO，可迁移到 Next.js
- 如果需要写功能，可添加 OAuth 登录
- 如果需要复杂状态管理，可引入 Zustand 或 Jotai
