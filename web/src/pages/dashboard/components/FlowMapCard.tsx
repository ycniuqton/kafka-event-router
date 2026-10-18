import { useMemo, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { AppPaths } from '../../../app/paths';
import { Card, EmptyState, Mono } from '../../../components/ui';
import { RouteDetailTab, type FlowMap } from '../../../domain';
import { services } from '../../../api';
import { useConnections, useRoutes, useServiceQuery } from '../../../store';
import { useLiveRefresh } from '../model/useRefreshEvery';
import { FlowFilterManager, type FlowFilterKey, type FlowFilterValues } from '../model/FlowFilters';
import { FlowGraphBuilder } from '../model/FlowGraph';
import { FlowLegend } from './FlowLegend';
import { FlowMapCanvas } from './FlowMapCanvas';
import { FlowMapFilters } from './FlowMapFilters';
import styles from './FlowMapCard.module.css';

/** Empty map while the first answer is loading. */
const FlowMapEmpty: FlowMap = { nodes: [], edges: [], metricsAvailable: true };

/** Signature visual: topics as boxes, routes as animated lines; hover names a route, click opens it. */
export function FlowMapCard() {
  const navigate = useNavigate();
  const routes = useRoutes();
  const connections = useConnections();
  const [filters, setFilters] = useState<FlowFilterValues>(FlowFilterManager.empty);
  const [hovered, setHovered] = useState<string | null>(null);
  const context = useMemo(() => ({ routes, connections }), [routes, connections]);
  const { data: map, reload } = useServiceQuery(() => services.metrics.flowMap(FlowFilterManager.query(filters)), [filters]);
  useLiveRefresh(reload);
  const graph = useMemo(() => FlowGraphBuilder.build(map ?? FlowMapEmpty, hovered), [map, hovered]);
  const setFilter = (key: FlowFilterKey, value: string) => setFilters((f) => ({ ...f, [key]: value }));
  return (
    <Card className={styles.card}>
      <div className={styles.header}>
        <div className={styles.title}>Flow map</div>
        <Mono className={styles.hover} truncate>
          {FlowGraphBuilder.hoverText(map, hovered)}
        </Mono>
        <FlowLegend />
      </div>
      <FlowMapFilters values={filters} context={context} onChange={setFilter} onReset={() => setFilters(FlowFilterManager.empty())} />
      <div className={styles.body}>
        {graph.edges.length ? (
          <FlowMapCanvas graph={graph} onHover={setHovered} onOpen={(id) => navigate(AppPaths.routeDetail(id, RouteDetailTab.Overview))} />
        ) : (
          <EmptyState compact title="No routes to draw" description="No deployed route matches these filters. Clear a filter, or create a route." />
        )}
      </div>
    </Card>
  );
}
