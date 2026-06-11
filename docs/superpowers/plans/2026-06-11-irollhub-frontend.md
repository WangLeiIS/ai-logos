# irollhub Frontend Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Create a React-based frontend for irollhub with browse, search, and view functionality for .iroll packages.

**Architecture:** Single-page application using Vite + React + TypeScript + TailwindCSS + React Router. Standalone deployment (static files) with CORS to irollhub API.

**Tech Stack:** Vite 6, React 19, TypeScript 5, React Router v7, TailwindCSS 4

---

## File Structure

```
irollhub-web/                          # New project root
├── index.html                          # HTML entry
├── package.json                        # Dependencies
├── tsconfig.json                       # TypeScript config
├── vite.config.ts                      # Vite config
├── tailwind.config.js                  # TailwindCSS config
├── postcss.config.js                   # PostCSS config
├── .env                                # Environment variables
├── src/
│   ├── main.tsx                        # React entry
│   ├── App.tsx                         # Root component + Router
│   ├── index.css                       # Global styles + Tailwind directives
│   ├── types/
│   │   └── index.ts                    # TypeScript types (Organization, Package, Version)
│   ├── api/
│   │   └── client.ts                   # API client functions
│   ├── components/
│   │   ├── layout/
│   │   │   ├── Header.tsx              # Navigation bar
│   │   │   └── Footer.tsx              # Footer
│   │   ├── home/
│   │   │   └── Hero.tsx                # Homepage search section
│   │   ├── package/
│   │   │   ├── PackageCard.tsx         # Package card component
│   │   │   ├── PackageList.tsx         # Package list grid
│   │   │   ├── PackageDetail.tsx       # Package detail view
│   │   │   └── VersionList.tsx        # Version list
│   │   ├── org/
│   │   │   ├── OrgCard.tsx             # Organization card
│   │   │   └── OrgList.tsx             # Organization list
│   │   └── search/
│   │       └── SearchBar.tsx           # Search input component
│   └── pages/
│       ├── HomePage.tsx                # Homepage route
│       ├── SearchPage.tsx              # Search results route
│       ├── OrgsPage.tsx                # Organizations list route
│       ├── OrgDetailPage.tsx           # Organization detail route
│       └── PackageDetailPage.tsx       # Package detail route
```

---

## Task 1: Create Vite + React + TypeScript Project

**Files:**
- Create: `irollhub-web/` directory and all scaffolded files

- [ ] **Step 1: Create Vite project**

Run from `ai-logos/` directory:

```bash
npm create vite@latest irollhub-web -- --template react-ts
cd irollhub-web
npm install
```

Expected output: Project created with `package.json`, `tsconfig.json`, `vite.config.ts`, `index.html`, `src/main.tsx`, `src/App.tsx`

- [ ] **Step 2: Verify project runs**

```bash
npm run dev
```

Expected: Dev server starts on http://localhost:5173, shows "Vite + React" page. Press Ctrl+C to stop.

- [ ] **Step 3: Commit initial scaffold**

```bash
cd irollhub-web
git init
git add .
git commit -m "feat: scaffold Vite React TypeScript project"
```

---

## Task 2: Install and Configure TailwindCSS

**Files:**
- Create: `irollhub-web/tailwind.config.js`
- Create: `irollhub-web/postcss.config.js`
- Modify: `irollhub-web/src/index.css`

- [ ] **Step 1: Install TailwindCSS dependencies**

```bash
cd irollhub-web
npm install -D tailwindcss postcss autoprefixer
npx tailwindcss init -p
```

Expected: Creates `tailwind.config.js` and `postcss.config.js`

- [ ] **Step 2: Configure TailwindCSS**

Create `irollhub-web/tailwind.config.js`:

```javascript
/** @type {import('tailwindcss').Config} */
export default {
  content: [
    "./index.html",
    "./src/**/*.{js,ts,jsx,tsx}",
  ],
  theme: {
    extend: {
      colors: {
        primary: '#1a1a1a',
        secondary: '#666666',
        accent: '#000000',
        border: '#e5e5e5',
      },
    },
  },
  plugins: [],
}
```

- [ ] **Step 3: Configure PostCSS**

Verify `irollhub-web/postcss.config.js` exists with:

```javascript
export default {
  plugins: {
    tailwindcss: {},
    autoprefixer: {},
  },
}
```

- [ ] **Step 4: Add TailwindCSS directives**

Replace `irollhub-web/src/index.css` content with:

```css
@tailwind base;
@tailwind components;
@tailwind utilities;

body {
  @apply bg-white text-gray-900;
}
```

- [ ] **Step 5: Test TailwindCSS is working**

Modify `irollhub-web/src/App.tsx` temporarily to:

```tsx
function App() {
  return (
    <div className="min-h-screen flex items-center justify-center">
      <h1 className="text-4xl font-bold text-primary">TailwindCSS Test</h1>
    </div>
  );
}

export default App;
```

Run: `npm run dev`  
Expected: Browser shows centered "TailwindCSS Test" in large bold text. Press Ctrl+C to stop.

- [ ] **Step 6: Restore App.tsx and commit**

Restore `irollhub-web/src/App.tsx` to original scaffold content:

```tsx
import { useState } from 'react'
import reactLogo from './assets/react.svg'
import viteLogo from '/vite.svg'
import './App.css'

function App() {
  const [count, setCount] = useState(0)

  return (
    <div className="min-h-screen">
      {/* original scaffold content */}
    </div>
  )
}

export default App
```

```bash
git add tailwind.config.js postcss.config.js src/index.css
git commit -m "feat: configure TailwindCSS"
```

---

## Task 3: Install React Router

**Files:**
- Modify: `irollhub-web/package.json`

- [ ] **Step 1: Install React Router**

```bash
cd irollhub-web
npm install react-router-dom
```

Expected: `react-router-dom` added to `package.json` dependencies

- [ ] **Step 2: Verify installation**

```bash
grep react-router-dom package.json
```

Expected: Line showing `"react-router-dom": "^7.x.x"`

- [ ] **Step 3: Commit**

```bash
git add package.json package-lock.json
git commit -m "feat: install React Router v7"
```

---

## Task 4: Create TypeScript Type Definitions

**Files:**
- Create: `irollhub-web/src/types/index.ts`

- [ ] **Step 1: Create type definitions file**

Create `irollhub-web/src/types/index.ts`:

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
  tags: string;  // JSON string array
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

export interface PackageDetail extends Package {
  versions: Version[];
}

export interface OrgDetail extends Organization {
  packages: Package[];
}

export interface SearchResponse {
  data: Array<{
    org: Organization;
    package: Package;
  }>;
}

export interface ApiError {
  error: string;
  code: string;
}
```

- [ ] **Step 2: Verify TypeScript compilation**

```bash
npm run build
```

Expected: Build succeeds, type errors reported if any (should be none)

- [ ] **Step 3: Commit**

```bash
git add src/types/
git commit -m "feat: add TypeScript type definitions"
```

---

## Task 5: Create API Client

**Files:**
- Create: `irollhub-web/src/api/client.ts`
- Create: `irollhub-web/.env`

- [ ] **Step 1: Create environment variables**

Create `irollhub-web/.env`:

```bash
VITE_API_BASE_URL=http://localhost:8080
```

- [ ] **Step 2: Create API client**

Create `irollhub-web/src/api/client.ts`:

```typescript
const API_BASE_URL = import.meta.env.VITE_API_BASE_URL || 'http://localhost:8080';

async function handleResponse<T>(response: Response): Promise<T> {
  if (!response.ok) {
    const error = await response.json().catch(() => ({ error: 'Unknown error', code: 'UNKNOWN' }));
    throw error;
  }
  return response.json();
}

export const api = {
  search: async (q: string) => {
    const response = await fetch(`${API_BASE_URL}/api/v1/search?q=${encodeURIComponent(q)}`);
    return handleResponse(response);
  },

  listOrgs: async (limit = 20, offset = 0) => {
    const response = await fetch(`${API_BASE_URL}/api/v1/orgs?limit=${limit}&offset=${offset}`);
    return handleResponse<{ data: import('../types').Organization[] }>(response);
  },

  getOrg: async (org: string) => {
    const response = await fetch(`${API_BASE_URL}/api/v1/orgs/${org}`);
    return handleResponse<import('../types').OrgDetail>(response);
  },

  getPackages: async (org: string) => {
    const response = await fetch(`${API_BASE_URL}/api/v1/orgs/${org}/packages`);
    return handleResponse<{ data: import('../types').Package[] }>(response);
  },

  getPackage: async (org: string, pkg: string) => {
    const response = await fetch(`${API_BASE_URL}/api/v1/orgs/${org}/packages/${pkg}`);
    return handleResponse<import('../types').PackageDetail>(response);
  },
};
```

- [ ] **Step 3: Verify TypeScript compilation**

```bash
npm run build
```

Expected: Build succeeds

- [ ] **Step 4: Commit**

```bash
git add src/api/ .env
git commit -m "feat: add API client with environment configuration"
```

---

## Task 6: Create Layout Components

**Files:**
- Create: `irollhub-web/src/components/layout/Header.tsx`
- Create: `irollhub-web/src/components/layout/Footer.tsx`

- [ ] **Step 1: Create Header component**

Create `irollhub-web/src/components/layout/Header.tsx`:

```tsx
import { Link, useNavigate } from 'react-router-dom';
import { useState } from 'react';
import { SearchBar } from '../search/SearchBar';

export function Header() {
  const navigate = useNavigate();

  const handleSearch = (query: string) => {
    if (query.trim()) {
      navigate(`/search?q=${encodeURIComponent(query)}`);
    }
  };

  return (
    <header className="bg-white border-b border-border sticky top-0 z-50">
      <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8">
        <div className="flex items-center justify-between h-16">
          <Link to="/" className="text-xl font-bold text-primary hover:opacity-70 transition-opacity">
            irollhub
          </Link>
          <div className="flex-1 max-w-md mx-8">
            <SearchBar onSearch={handleSearch} />
          </div>
          <nav className="flex items-center space-x-6">
            <Link to="/orgs" className="text-secondary hover:text-primary transition-colors">
              Organizations
            </Link>
          </nav>
        </div>
      </div>
    </header>
  );
}
```

- [ ] **Step 2: Create Footer component**

Create `irollhub-web/src/components/layout/Footer.tsx`:

```tsx
export function Footer() {
  return (
    <footer className="bg-white border-t border-border mt-auto">
      <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-6">
        <p className="text-center text-secondary text-sm">
          © 2026 irollhub. AI Agent Package Registry.
        </p>
      </div>
    </footer>
  );
}
```

- [ ] **Step 3: Verify TypeScript compilation**

```bash
npm run build
```

Expected: Build succeeds (SearchBar component not created yet will cause error, fix in next task)

- [ ] **Step 4: Commit**

```bash
git add src/components/layout/
git commit -m "feat: add Header and Footer layout components"
```

---

## Task 7: Create SearchBar Component

**Files:**
- Create: `irollhub-web/src/components/search/SearchBar.tsx`

- [ ] **Step 1: Create SearchBar component**

Create `irollhub-web/src/components/search/SearchBar.tsx`:

```tsx
import { useState, FormEvent } from 'react';

interface SearchBarProps {
  onSearch: (query: string) => void;
  defaultValue?: string;
}

export function SearchBar({ onSearch, defaultValue }: SearchBarProps) {
  const [query, setQuery] = useState(defaultValue || '');

  const handleSubmit = (e: FormEvent) => {
    e.preventDefault();
    onSearch(query);
  };

  return (
    <form onSubmit={handleSubmit} className="w-full">
      <input
        type="text"
        value={query}
        onChange={(e) => setQuery(e.target.value)}
        placeholder="Search packages..."
        className="w-full px-4 py-2 border border-border rounded-lg bg-white text-primary placeholder:text-secondary focus:outline-none focus:ring-2 focus:ring-accent focus:border-transparent transition-all"
      />
    </form>
  );
}
```

- [ ] **Step 2: Verify TypeScript compilation**

```bash
npm run build
```

Expected: Build succeeds

- [ ] **Step 3: Commit**

```bash
git add src/components/search/
git commit -m "feat: add SearchBar component"
```

---

## Task 8: Create Organization Components

**Files:**
- Create: `irollhub-web/src/components/org/OrgCard.tsx`
- Create: `irollhub-web/src/components/org/OrgList.tsx`

- [ ] **Step 1: Create OrgCard component**

Create `irollhub-web/src/components/org/OrgCard.tsx`:

```tsx
import { Link } from 'react-router-dom';
import type { Organization } from '../../types';

interface OrgCardProps {
  org: Organization;
}

export function OrgCard({ org }: OrgCardProps) {
  return (
    <Link
      to={`/orgs/${org.name}`}
      className="block p-6 bg-white border border-border rounded-lg hover:-translate-y-1 hover:shadow-md transition-all duration-200"
    >
      <div className="flex items-center space-x-4">
        <div className="w-12 h-12 rounded-full bg-secondary flex items-center justify-center text-white font-bold text-lg">
          {org.name.charAt(0).toUpperCase()}
        </div>
        <div>
          <h3 className="text-lg font-semibold text-primary">{org.name}</h3>
          <p className="text-sm text-secondary">
            Created {new Date(org.created_at).toLocaleDateString()}
          </p>
        </div>
      </div>
    </Link>
  );
}
```

- [ ] **Step 2: Create OrgList component**

Create `irollhub-web/src/components/org/OrgList.tsx`:

```tsx
import type { Organization } from '../../types';
import { OrgCard } from './OrgCard';

interface OrgListProps {
  orgs: Organization[];
}

export function OrgList({ orgs }: OrgListProps) {
  if (orgs.length === 0) {
    return (
      <div className="text-center py-12">
        <p className="text-secondary">No organizations found.</p>
      </div>
    );
  }

  return (
    <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
      {orgs.map((org) => (
        <OrgCard key={org.id} org={org} />
      ))}
    </div>
  );
}
```

- [ ] **Step 3: Verify TypeScript compilation**

```bash
npm run build
```

Expected: Build succeeds

- [ ] **Step 4: Commit**

```bash
git add src/components/org/
git commit -m "feat: add OrgCard and OrgList components"
```

---

## Task 9: Create Package Components

**Files:**
- Create: `irollhub-web/src/components/package/PackageCard.tsx`
- Create: `irollhub-web/src/components/package/PackageList.tsx`
- Create: `irollhub-web/src/components/package/VersionList.tsx`
- Create: `irollhub-web/src/components/package/PackageDetail.tsx`

- [ ] **Step 1: Create PackageCard component**

Create `irollhub-web/src/components/package/PackageCard.tsx`:

```tsx
import { Link } from 'react-router-dom';
import type { Package } from '../../types';

interface PackageCardProps {
  org: string;
  pkg: Package;
}

export function PackageCard({ org, pkg }: PackageCardProps) {
  const tags = JSON.parse(pkg.tags || '[]');

  return (
    <Link
      to={`/orgs/${org}/packages/${pkg.name}`}
      className="block p-6 bg-white border border-border rounded-lg hover:-translate-y-1 hover:shadow-md transition-all duration-200"
    >
      <h3 className="text-lg font-semibold text-primary mb-2">{pkg.name}</h3>
      <p className="text-secondary text-sm mb-4 line-clamp-2">{pkg.description || 'No description'}</p>
      <div className="flex items-center justify-between">
        <div className="flex space-x-2">
          {tags.slice(0, 3).map((tag: string) => (
            <span
              key={tag}
              className="px-2 py-1 text-xs bg-secondary text-white rounded"
            >
              {tag}
            </span>
          ))}
        </div>
        <span className="text-xs text-secondary">{pkg.downloads} downloads</span>
      </div>
    </Link>
  );
}
```

- [ ] **Step 2: Create PackageList component**

Create `irollhub-web/src/components/package/PackageList.tsx`:

```tsx
import type { Package } from '../../types';
import { PackageCard } from './PackageCard';

interface PackageListProps {
  org: string;
  packages: Package[];
}

export function PackageList({ org, packages }: PackageListProps) {
  if (packages.length === 0) {
    return (
      <div className="text-center py-12">
        <p className="text-secondary">No packages found.</p>
      </div>
    );
  }

  return (
    <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
      {packages.map((pkg) => (
        <PackageCard key={pkg.id} org={org} pkg={pkg} />
      ))}
    </div>
  );
}
```

- [ ] **Step 3: Create VersionList component**

Create `irollhub-web/src/components/package/VersionList.tsx`:

```tsx
import type { Version } from '../../types';

interface VersionListProps {
  versions: Version[];
}

export function VersionList({ versions }: VersionListProps) {
  if (versions.length === 0) {
    return (
      <div className="text-center py-8">
        <p className="text-secondary">No versions available.</p>
      </div>
    );
  }

  return (
    <div className="space-y-2">
      {versions.map((version) => (
        <div
          key={version.id}
          className="p-4 bg-white border border-border rounded-lg"
        >
          <div className="flex items-center justify-between">
            <div>
              <span className="font-semibold text-primary">{version.version}</span>
              <span className="ml-4 text-sm text-secondary">
                {new Date(version.created_at).toLocaleDateString()}
              </span>
            </div>
            <span className="text-sm text-secondary">
              {(version.file_size / 1024 / 1024).toFixed(2)} MB
            </span>
          </div>
        </div>
      ))}
    </div>
  );
}
```

- [ ] **Step 4: Create PackageDetail component**

Create `irollhub-web/src/components/package/PackageDetail.tsx`:

```tsx
import type { PackageDetail } from '../../types';
import { VersionList } from './VersionList';

interface PackageDetailProps {
  org: string;
  pkg: PackageDetail;
}

export function PackageDetail({ org, pkg }: PackageDetailProps) {
  const tags = JSON.parse(pkg.tags || '[]');

  return (
    <div className="space-y-8">
      <div>
        <h1 className="text-3xl font-bold text-primary mb-4">{pkg.name}</h1>
        <p className="text-secondary mb-4">{pkg.description || 'No description'}</p>
        <div className="flex space-x-2">
          {tags.map((tag: string) => (
            <span
              key={tag}
              className="px-3 py-1 text-sm bg-secondary text-white rounded"
            >
              {tag}
            </span>
          ))}
        </div>
      </div>

      <div>
        <h2 className="text-xl font-semibold text-primary mb-4">Versions</h2>
        <VersionList versions={pkg.versions} />
      </div>

      <div className="text-sm text-secondary">
        <p>Created: {new Date(pkg.created_at).toLocaleString()}</p>
        <p>Updated: {new Date(pkg.updated_at).toLocaleString()}</p>
        <p>Downloads: {pkg.downloads}</p>
      </div>
    </div>
  );
}
```

- [ ] **Step 5: Verify TypeScript compilation**

```bash
npm run build
```

Expected: Build succeeds

- [ ] **Step 6: Commit**

```bash
git add src/components/package/
git commit -m "feat: add PackageCard, PackageList, VersionList, PackageDetail components"
```

---

## Task 10: Create Hero Component

**Files:**
- Create: `irollhub-web/src/components/home/Hero.tsx`

- [ ] **Step 1: Create Hero component**

Create `irollhub-web/src/components/home/Hero.tsx`:

```tsx
import { useState, FormEvent } from 'react';
import { useNavigate } from 'react-router-dom';

export function Hero() {
  const navigate = useNavigate();
  const [query, setQuery] = useState('');

  const handleSubmit = (e: FormEvent) => {
    e.preventDefault();
    if (query.trim()) {
      navigate(`/search?q=${encodeURIComponent(query)}`);
    }
  };

  return (
    <div className="bg-gradient-to-b from-gray-50 to-white py-20">
      <div className="max-w-3xl mx-auto px-4 text-center">
        <h1 className="text-5xl font-bold text-primary mb-6">
          irollhub
        </h1>
        <p className="text-xl text-secondary mb-8">
          AI Agent Package Registry
        </p>
        <form onSubmit={handleSubmit} className="max-w-xl mx-auto">
          <input
            type="text"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder="Search for packages..."
            className="w-full px-6 py-4 text-lg border border-border rounded-lg bg-white focus:outline-none focus:ring-2 focus:ring-accent focus:border-transparent transition-all"
          />
        </form>
      </div>
    </div>
  );
}
```

- [ ] **Step 2: Verify TypeScript compilation**

```bash
npm run build
```

Expected: Build succeeds

- [ ] **Step 3: Commit**

```bash
git add src/components/home/
git commit -m "feat: add Hero component for homepage"
```

---

## Task 11: Create Page Components

**Files:**
- Create: `irollhub-web/src/pages/HomePage.tsx`
- Create: `irollhub-web/src/pages/SearchPage.tsx`
- Create: `irollhub-web/src/pages/OrgsPage.tsx`
- Create: `irollhub-web/src/pages/OrgDetailPage.tsx`
- Create: `irollhub-web/src/pages/PackageDetailPage.tsx`

- [ ] **Step 1: Create HomePage**

Create `irollhub-web/src/pages/HomePage.tsx`:

```tsx
import { Hero } from '../components/home/Hero';
import { OrgList } from '../components/org/OrgList';
import { api } from '../api/client';
import { useQuery } from '@tanstack/react-query';

export function HomePage() {
  const { data: orgsData, isLoading, error } = useQuery({
    queryKey: ['orgs'],
    queryFn: () => api.listOrgs(6, 0),
  });

  const orgs = orgsData?.data || [];

  return (
    <div className="min-h-screen">
      <Hero />
      <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-12">
        <h2 className="text-2xl font-bold text-primary mb-6">Featured Organizations</h2>
        {isLoading ? (
          <p className="text-secondary">Loading...</p>
        ) : error ? (
          <p className="text-red-500">Error loading organizations</p>
        ) : (
          <OrgList orgs={orgs} />
        )}
      </div>
    </div>
  );
}
```

- [ ] **Step 2: Create SearchPage**

Create `irollhub-web/src/pages/SearchPage.tsx`:

```tsx
import { useSearchParams } from 'react-router-dom';
import { PackageList } from '../components/package/PackageList';
import { api } from '../api/client';
import { useQuery } from '@tanstack/react-query';

export function SearchPage() {
  const [searchParams] = useSearchParams();
  const query = searchParams.get('q') || '';

  const { data, isLoading, error } = useQuery({
    queryKey: ['search', query],
    queryFn: () => api.search(query),
    enabled: query.length > 0,
  });

  const results = data?.data || [];

  return (
    <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-12">
      <h1 className="text-3xl font-bold text-primary mb-6">
        Search results for "{query}"
      </h1>
      {isLoading ? (
        <p className="text-secondary">Loading...</p>
      ) : error ? (
        <p className="text-red-500">Error searching packages</p>
      ) : results.length === 0 ? (
        <p className="text-secondary">No results found.</p>
      ) : (
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
          {results.map(({ org, package: pkg }) => (
            <div key={`${org.id}-${pkg.id}`}>
              <PackageList org={org.name} packages={[pkg]} />
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
```

- [ ] **Step 3: Create OrgsPage**

Create `irollhub-web/src/pages/OrgsPage.tsx`:

```tsx
import { OrgList } from '../components/org/OrgList';
import { api } from '../api/client';
import { useQuery } from '@tanstack/react-query';

export function OrgsPage() {
  const { data, isLoading, error } = useQuery({
    queryKey: ['orgs'],
    queryFn: () => api.listOrgs(100, 0),
  });

  const orgs = data?.data || [];

  return (
    <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-12">
      <h1 className="text-3xl font-bold text-primary mb-6">Organizations</h1>
      {isLoading ? (
        <p className="text-secondary">Loading...</p>
      ) : error ? (
        <p className="text-red-500">Error loading organizations</p>
      ) : (
        <OrgList orgs={orgs} />
      )}
    </div>
  );
}
```

- [ ] **Step 4: Create OrgDetailPage**

Create `irollhub-web/src/pages/OrgDetailPage.tsx`:

```tsx
import { useParams } from 'react-router-dom';
import { PackageList } from '../components/package/PackageList';
import { api } from '../api/client';
import { useQuery } from '@tanstack/react-query';

export function OrgDetailPage() {
  const { org } = useParams<{ org: string }>();

  const { data, isLoading, error } = useQuery({
    queryKey: ['org', org],
    queryFn: () => api.getOrg(org || ''),
    enabled: !!org,
  });

  if (!org) return <div>Organization not found</div>;

  if (isLoading) return <div className="max-w-7xl mx-auto px-4 py-12"><p className="text-secondary">Loading...</p></div>;
  if (error) return <div className="max-w-7xl mx-auto px-4 py-12"><p className="text-red-500">Error loading organization</p></div>;
  if (!data) return <div className="max-w-7xl mx-auto px-4 py-12"><p className="text-secondary">Organization not found</p></div>;

  return (
    <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-12">
      <div className="mb-8">
        <h1 className="text-3xl font-bold text-primary mb-2">{data.name}</h1>
        <p className="text-secondary">
          Member since {new Date(data.created_at).toLocaleDateString()}
        </p>
      </div>
      <h2 className="text-2xl font-semibold text-primary mb-6">Packages</h2>
      <PackageList org={org} packages={data.packages || []} />
    </div>
  );
}
```

- [ ] **Step 5: Create PackageDetailPage**

Create `irollhub-web/src/pages/PackageDetailPage.tsx`:

```tsx
import { useParams } from 'react-router-dom';
import { PackageDetail } from '../components/package/PackageDetail';
import { api } from '../api/client';
import { useQuery } from '@tanstack/react-query';

export function PackageDetailPage() {
  const { org, pkg } = useParams<{ org: string; pkg: string }>();

  const { data, isLoading, error } = useQuery({
    queryKey: ['package', org, pkg],
    queryFn: () => api.getPackage(org || '', pkg || ''),
    enabled: !!org && !!pkg,
  });

  if (!org || !pkg) return <div>Package not found</div>;

  if (isLoading) return <div className="max-w-7xl mx-auto px-4 py-12"><p className="text-secondary">Loading...</p></div>;
  if (error) return <div className="max-w-7xl mx-auto px-4 py-12"><p className="text-red-500">Error loading package</p></div>;
  if (!data) return <div className="max-w-7xl mx-auto px-4 py-12"><p className="text-secondary">Package not found</p></div>;

  return (
    <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-12">
      <PackageDetail org={org} pkg={data} />
    </div>
  );
}
```

- [ ] **Step 6: Install TanStack Query dependency**

```bash
cd irollhub-web
npm install @tanstack/react-query
```

- [ ] **Step 7: Verify TypeScript compilation**

```bash
npm run build
```

Expected: Build succeeds

- [ ] **Step 8: Commit**

```bash
git add src/pages/ package.json package-lock.json
git commit -m "feat: add all page components with TanStack Query"
```

---

## Task 12: Configure App Router and Layout

**Files:**
- Modify: `irollhub-web/src/App.tsx`

- [ ] **Step 1: Replace App.tsx with Router configuration**

Replace `irollhub-web/src/App.tsx` content with:

```tsx
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { BrowserRouter, Routes, Route } from 'react-router-dom';
import { Header } from './components/layout/Header';
import { Footer } from './components/layout/Footer';
import { HomePage } from './pages/HomePage';
import { SearchPage } from './pages/SearchPage';
import { OrgsPage } from './pages/OrgsPage';
import { OrgDetailPage } from './pages/OrgDetailPage';
import { PackageDetailPage } from './pages/PackageDetailPage';
import './index.css';

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 5 * 60 * 1000,  // 5 minutes
      retry: 1,
    },
  },
});

function App() {
  return (
    <QueryClientProvider client={queryClient}>
      <BrowserRouter>
        <div className="min-h-screen flex flex-col">
          <Header />
          <main className="flex-1">
            <Routes>
              <Route path="/" element={<HomePage />} />
              <Route path="/search" element={<SearchPage />} />
              <Route path="/orgs" element={<OrgsPage />} />
              <Route path="/orgs/:org" element={<OrgDetailPage />} />
              <Route path="/orgs/:org/packages/:pkg" element={<PackageDetailPage />} />
            </Routes>
          </main>
          <Footer />
        </div>
      </BrowserRouter>
    </QueryClientProvider>
  );
}

export default App;
```

- [ ] **Step 2: Remove unused App.css**

```bash
rm src/App.css
```

- [ ] **Step 3: Verify TypeScript compilation**

```bash
npm run build
```

Expected: Build succeeds

- [ ] **Step 4: Test the application**

```bash
npm run dev
```

Expected:
- Browser shows http://localhost:5173 with Hero section and organizations
- Navigation works between pages
- Search bar routes to /search
- Clicking org cards navigates to org detail
- Clicking package cards navigates to package detail

Press Ctrl+C to stop.

- [ ] **Step 5: Commit**

```bash
git add src/App.tsx
git rm src/App.css 2>/dev/null || true
git add -A
git commit -m "feat: configure React Router with all routes and layout"
```

---

## Task 13: Add Error Handling and Loading States

**Files:**
- Modify: `irollhub-web/src/api/client.ts`

- [ ] **Step 1: Enhance API client with better error handling**

Replace `irollhub-web/src/api/client.ts` content with:

```typescript
const API_BASE_URL = import.meta.env.VITE_API_BASE_URL || 'http://localhost:8080';

export class ApiError extends Error {
  constructor(public message: string, public code: string) {
    super(message);
    this.name = 'ApiError';
  }
}

async function handleResponse<T>(response: Response): Promise<T> {
  if (!response.ok) {
    const error = await response.json().catch(() => ({ 
      error: 'Network error or invalid response', 
      code: 'NETWORK_ERROR' 
    }));
    throw new ApiError(error.error || 'Unknown error', error.code || 'UNKNOWN');
  }
  return response.json();
}

export const api = {
  search: async (q: string) => {
    const response = await fetch(`${API_BASE_URL}/api/v1/search?q=${encodeURIComponent(q)}`);
    return handleResponse(response);
  },

  listOrgs: async (limit = 20, offset = 0) => {
    const response = await fetch(`${API_BASE_URL}/api/v1/orgs?limit=${limit}&offset=${offset}`);
    return handleResponse<{ data: import('../types').Organization[] }>(response);
  },

  getOrg: async (org: string) => {
    const response = await fetch(`${API_BASE_URL}/api/v1/orgs/${org}`);
    return handleResponse<import('../types').OrgDetail>(response);
  },

  getPackages: async (org: string) => {
    const response = await fetch(`${API_BASE_URL}/api/v1/orgs/${org}/packages`);
    return handleResponse<{ data: import('../types').Package[] }>(response);
  },

  getPackage: async (org: string, pkg: string) => {
    const response = await fetch(`${API_BASE_URL}/api/v1/orgs/${org}/packages/${pkg}`);
    return handleResponse<import('../types').PackageDetail>(response);
  },
};
```

- [ ] **Step 2: Add global error boundary**

Create `irollhub-web/src/components/ErrorBoundary.tsx`:

```tsx
import { Component, ReactNode } from 'react';

interface Props {
  children: ReactNode;
}

interface State {
  hasError: boolean;
  error?: Error;
}

export class ErrorBoundary extends Component<Props, State> {
  constructor(props: Props) {
    super(props);
    this.state = { hasError: false };
  }

  static getDerivedStateFromError(error: Error): State {
    return { hasError: true, error };
  }

  render() {
    if (this.state.hasError) {
      return (
        <div className="min-h-screen flex items-center justify-center bg-white">
          <div className="text-center">
            <h1 className="text-2xl font-bold text-primary mb-4">Something went wrong</h1>
            <p className="text-secondary mb-4">{this.state.error?.message}</p>
            <button
              onClick={() => window.location.reload()}
              className="px-4 py-2 bg-accent text-white rounded hover:opacity-90 transition-opacity"
            >
              Reload
            </button>
          </div>
        </div>
      );
    }

    return this.props.children;
  }
}
```

- [ ] **Step 3: Update App.tsx to use ErrorBoundary**

Modify `irollhub-web/src/App.tsx`:

```tsx
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { BrowserRouter, Routes, Route } from 'react-router-dom';
import { Header } from './components/layout/Header';
import { Footer } from './components/layout/Footer';
import { HomePage } from './pages/HomePage';
import { SearchPage } from './pages/SearchPage';
import { OrgsPage } from './pages/OrgsPage';
import { OrgDetailPage } from './pages/OrgDetailPage';
import { PackageDetailPage } from './pages/PackageDetailPage';
import { ErrorBoundary } from './components/ErrorBoundary';
import './index.css';

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 5 * 60 * 1000,
      retry: 1,
    },
  },
});

function App() {
  return (
    <ErrorBoundary>
      <QueryClientProvider client={queryClient}>
        <BrowserRouter>
          <div className="min-h-screen flex flex-col">
            <Header />
            <main className="flex-1">
              <Routes>
                <Route path="/" element={<HomePage />} />
                <Route path="/search" element={<SearchPage />} />
                <Route path="/orgs" element={<OrgsPage />} />
                <Route path="/orgs/:org" element={<OrgDetailPage />} />
                <Route path="/orgs/:org/packages/:pkg" element={<PackageDetailPage />} />
              </Routes>
            </main>
            <Footer />
          </div>
        </BrowserRouter>
      </QueryClientProvider>
    </ErrorBoundary>
  );
}

export default App;
```

- [ ] **Step 4: Verify TypeScript compilation**

```bash
npm run build
```

Expected: Build succeeds

- [ ] **Step 5: Commit**

```bash
git add src/
git commit -m "feat: add ErrorBoundary and enhance API error handling"
```

---

## Task 14: Final Polish and Production Build

**Files:**
- Modify: `irollhub-web/package.json`
- Create: `irollhub-web/README.md`

- [ ] **Step 1: Add build and preview scripts**

Verify `irollhub-web/package.json` has these scripts (should already exist):

```json
{
  "scripts": {
    "dev": "vite",
    "build": "tsc -b && vite build",
    "preview": "vite preview"
  }
}
```

- [ ] **Step 2: Create README**

Create `irollhub-web/README.md`:

```markdown
# irollhub Frontend

React frontend for irollhub - AI Agent Package Registry.

## Development

\`\`\`bash
npm install
npm run dev
\`\`\`

Open http://localhost:5173

## Build

\`\`\`bash
npm run build
\`\`\`

Output in `dist/` directory.

## Environment Variables

Create `.env` file:

\`\`\`
VITE_API_BASE_URL=http://localhost:8080
\`\`\`

## Deployment

Deploy `dist/` directory to any static hosting service.
```

- [ ] **Step 3: Final build test**

```bash
cd irollhub-web
npm run build
npm run preview
```

Expected: Production build succeeds, preview server starts on http://localhost:4173, app works correctly. Press Ctrl+C to stop.

- [ ] **Step 4: Commit README and final changes**

```bash
cd irollhub-web
git add README.md
git commit -m "docs: add README with deployment instructions"
```

- [ ] **Step 5: Verify git history**

```bash
git log --oneline
```

Expected: Series of 14 commits showing implementation progression

---

## Self-Review Checklist

- [ ] All spec requirements implemented: Browse, Search, View organizations/packages/versions
- [ ] No placeholders or TBD in code blocks
- [ ] All file paths are specific and absolute (relative to irollhub-web/)
- [ ] TypeScript types consistent across all files
- [ ] TailwindCSS classes match design spec (简约现代风格)
- [ ] API endpoints match irollhub handler routes
- [ ] All steps include expected output/verification
- [ ] Git commits follow pattern and are atomic

---

## Next Steps After Implementation

1. Configure CORS in irollhub Go service
2. Deploy frontend to static hosting (Vercel, Netlify, Nginx)
3. Test against live irollhub API
4. Add analytics/monitoring if needed
