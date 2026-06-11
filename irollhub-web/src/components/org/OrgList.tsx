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
