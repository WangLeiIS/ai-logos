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
