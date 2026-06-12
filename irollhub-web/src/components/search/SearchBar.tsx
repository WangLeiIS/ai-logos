import { useState } from 'react';
import type { FormEvent } from 'react';

interface SearchBarProps {
  onSearch: (query: string) => void;
  defaultValue?: string;
  placeholder?: string;
  size?: 'sm' | 'lg';
}

export function SearchBar({ onSearch, defaultValue, placeholder = 'Search packages…', size = 'sm' }: SearchBarProps) {
  const [query, setQuery] = useState(defaultValue || '');

  const handleSubmit = (e: FormEvent) => {
    e.preventDefault();
    onSearch(query);
  };

  const sizeClasses = size === 'lg'
    ? 'px-6 py-4 text-lg'
    : 'px-4 py-2';

  return (
    <form onSubmit={handleSubmit} className="w-full" role="search">
      <label htmlFor="search-input" className="sr-only">Search packages</label>
      <div className="relative">
        <input
          id="search-input"
          type="search"
          name="q"
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          placeholder={placeholder}
          autoComplete="off"
          spellCheck={false}
          className={`w-full ${sizeClasses} border border-border rounded-lg bg-white text-primary placeholder:text-secondary focus:outline-none focus-visible:ring-2 focus-visible:ring-accent focus-visible:border-transparent transition-shadow`}
        />
        <button
          type="submit"
          aria-label="Search"
          className="absolute right-2 top-1/2 -translate-y-1/2 p-2 text-secondary hover:text-primary transition-colors"
        >
          <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
            <circle cx="11" cy="11" r="8" />
            <line x1="21" y1="21" x2="16.65" y2="16.65" />
          </svg>
        </button>
      </div>
    </form>
  );
}
