import { useNavigate } from 'react-router-dom';
import { SearchBar } from '../search/SearchBar';

export function Hero() {
  const navigate = useNavigate();

  const handleSearch = (query: string) => {
    if (query.trim()) {
      navigate(`/search?q=${encodeURIComponent(query)}`);
    }
  };

  return (
    <div className="bg-linear-to-b from-gray-50 to-white py-20">
      <div className="max-w-3xl mx-auto px-4 text-center">
        <h1 className="text-5xl font-bold text-primary mb-6">
          irollhub
        </h1>
        <p className="text-xl text-secondary mb-8">
          AI Agent Package Registry
        </p>
        <div className="max-w-xl mx-auto">
          <SearchBar onSearch={handleSearch} placeholder="Search for packages…" size="lg" />
        </div>
      </div>
    </div>
  );
}
