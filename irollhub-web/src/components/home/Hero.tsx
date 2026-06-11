import { useState } from 'react';
import type { FormEvent } from 'react';
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
