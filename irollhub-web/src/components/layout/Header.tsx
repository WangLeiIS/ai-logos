import { Link, useNavigate } from 'react-router-dom';
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
