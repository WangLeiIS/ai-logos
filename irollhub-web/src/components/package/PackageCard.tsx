import { Link } from 'react-router-dom';
import type { Package } from '../../types';
import { parseTags } from '../../types';

interface PackageCardProps {
  org: string;
  pkg: Package;
}

export function PackageCard({ org, pkg }: PackageCardProps) {
  const tags = parseTags(pkg.tags);

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
