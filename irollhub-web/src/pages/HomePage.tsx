import { Hero } from '../components/home/Hero';
import { OrgList } from '../components/org/OrgList';
import { api } from '../api/client';
import { useQuery } from '@tanstack/react-query';

export function HomePage() {
  const { data: orgsData, isLoading, error } = useQuery({
    queryKey: ['orgs', { limit: 6, offset: 0 }],
    queryFn: () => api.listOrgs(6, 0),
  });

  const orgs = orgsData?.data || [];

  return (
    <div className="min-h-screen">
      <Hero />
      <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-12">
        <h2 className="text-2xl font-bold text-primary mb-6">Featured Organizations</h2>
        {isLoading ? (
          <p className="text-secondary" aria-live="polite">Loading…</p>
        ) : error ? (
          <p className="text-red-500" role="alert">Error loading organizations</p>
        ) : (
          <OrgList orgs={orgs} />
        )}
      </div>
    </div>
  );
}
