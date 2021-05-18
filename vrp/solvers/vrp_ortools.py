from ortools.constraint_solver import pywrapcp
from ortools.constraint_solver import routing_enums_pb2
from decimal import Decimal
import math
import utils
import sys
import json
import collections
import multiprocessing as mp

FIRST_SOLUTION_HEURISTICS = {
    'path_cheapest_arc':
    routing_enums_pb2.FirstSolutionStrategy.PATH_CHEAPEST_ARC,
    'path_most_constrained_arc':
    routing_enums_pb2.FirstSolutionStrategy.PATH_MOST_CONSTRAINED_ARC,
    'christophides':
    routing_enums_pb2.FirstSolutionStrategy.CHRISTOFIDES,
    'all_unperformed':
    routing_enums_pb2.FirstSolutionStrategy.ALL_UNPERFORMED,
    'best_insertion':
    routing_enums_pb2.FirstSolutionStrategy.BEST_INSERTION,
    'parallel_cheapest_insertion':
    routing_enums_pb2.FirstSolutionStrategy.PARALLEL_CHEAPEST_INSERTION,
    'local_cheapest_insertion':
    routing_enums_pb2.FirstSolutionStrategy.LOCAL_CHEAPEST_INSERTION,
    'global_cheapest_arc':
    routing_enums_pb2.FirstSolutionStrategy.GLOBAL_CHEAPEST_ARC,
    'local_cheapest_arc':
    routing_enums_pb2.FirstSolutionStrategy.LOCAL_CHEAPEST_ARC,
    'first_unbound_min_value':
    routing_enums_pb2.FirstSolutionStrategy.FIRST_UNBOUND_MIN_VALUE,
}

CAPACITY_TASK_BIAS = 700

class VRPSolver:
    def __init__(self,
                 im,
                 drones,
                 budget,
                 capacity=None,
                 unweighted_im=None,
                 initial_schedule=None,
                 rth=None,
                 dist_mat=None,
                 local_search=False,
                 verbose=False):
        def schedule_to_routes(schedule):
            routes = []
            for r in schedule['routes']:
                route = []
                for task in r['path']:
                    route += [(task['location']['latitude'], 
                                task['location']['longitude'],
                               task['app_id'], task['request_time'])]
                routes += [route]
            return routes

        self.interest_map = im
        self.unweighted_im = unweighted_im
        self.drones = drones
        self.capacity = capacity
        self.budget = budget + self.capacity * CAPACITY_TASK_BIAS if self.capacity else budget
        self.initial_routes = schedule_to_routes(
            initial_schedule) if initial_schedule else None
        self.local_search = local_search
        self.verbose = verbose
        self.rth = rth
        self.dist_mat = dist_mat

    def solve(self, heuristic):
        def generate_distance_matrix(locs):
            if self.dist_mat == None:
                dim = len(locs)

                # initialize distance matrix to zeros
                d = [[0 for i in range(dim)] for j in range(dim)]

                # compute distances between actual nodes
                for i in range(dim):
                    for j in range(dim):
                        if locs[i] == (-1, -1, -1, -1) or locs[j] == (-1, -1, -1, -1):
                            d[i][j] = 0.0
                        else:
                            dist = utils.travel_time(
                                locs[i], locs[j], self.drones[0]['speed'],
                                self.interest_map[locs[j]]['task_time_seconds']
                                if locs[j] in self.interest_map else 0.0)
                            d[i][j] = dist

                return d
            else:
                dim = len(locs)

                # initialize distance matrix to zeros
                d = [[0 for i in range(dim)] for j in range(dim)]

                # compute distances between actual nodes
                for i in range(dim):
                    for j in range(dim):
                        if locs[i] == (-1, -1, -1, -1) or locs[j] == (-1, -1, -1, -1):
                            d[i][j] = 0.0
                        elif locs[i] == locs[j]:
                            d[i][j] = self.interest_map[locs[j]]['task_time_seconds'] \
                                    if locs[j] in self.interest_map else 0.0
                        else:
                            x = self.dist_mat[(locs[i][0:2], locs[j][0:2])]
                            if math.isnan(x):
                                d[i][j] = 1000000
                            else:
                                d[i][j] = int(x)
                            d[i][j] += self.interest_map[locs[j]]['task_time_seconds'] \
                                    if locs[j] in self.interest_map else 0.0
               
                return d

        def create_data_model(cells, initial_routes=None):
            start = [
                cells.index(
                    (d['location']['latitude'], d['location']['longitude'], -1, -1)
                ) for d in self.drones
            ]
            end = [cells.index((depot[0], depot[1], -1, -1)) for depot in self.rth]\
                if self.rth else [0 for i in range(len(self.drones))]
            data = {
                'distances': generate_distance_matrix(cells),
                'num_locations': len(cells),
                'num_vehicles': len(self.drones),
                'start_locations': start,
                'end_locations': end
            }
            if initial_routes:
                data['initial_routes'] = [[
                    cells.index(t) if self.rth else cells.index(t) - 1
                    for t in r
                ] for r in initial_routes]
            return data

        def create_distance_callback(manager, data, locs):
            distances = data['distances']

            def distance_callback(src, dst):
                from_node = manager.IndexToNode(src)
                to_node = manager.IndexToNode(dst)
                return distances[from_node][to_node]

            return distance_callback

        def add_distance_dimension(routing, transit_callback_instance,
                                   num_vehicles):
            distance = 'Distance'
            maximum_distance = int(self.budget)  # seconds
            routing.AddDimension(
                transit_callback_instance,
                0,  # no slack
                maximum_distance,
                True,  # start cumul to zero
                distance)

        def parse_solution(data, manager, routing, solution, locs):
            m = generate_distance_matrix(locs)
            routes = [dict() for x in range(data['num_vehicles'])]
            for vehicle_id in range(data['num_vehicles']):
                index = routing.Start(vehicle_id)
                route_time = 0
                total_interest = 0.0
                routes[vehicle_id] = {
                    'path': [],
                    'total_interest': 0.0,
                    'total_time': 0,
                    'vehicle_start': [0.0, 0.0],
                    'vehicle_end': [0.0, 0.0]
                }

                cost = 0.0
                while not routing.IsEnd(index):
                    node = manager.IndexToNode(index)
                    if locs[node] != (-1, -1, -1, -1):
                        routes[vehicle_id]['path'] += [{
                            'location': {'latitude': locs[node][0], 'longitude': locs[node][1]},
                            'app_id': locs[node][2],
                            'request_time': locs[node][3],
                            'fulfill_time': int(cost)
                        }]

                    # loc has interest if not associated with app (i.e. it is a drone start loc)
                    if locs[node][2] != -1:
                        total_interest += self.unweighted_im[locs[node]]['interest'] if self.unweighted_im \
                            else self.interest_map[locs[node]]['interest']
                    previous_index = index
                    index = solution.Value(routing.NextVar(index))

                    route_time += routing.GetArcCostForVehicle(
                        previous_index, index, vehicle_id)

                    prev_node = locs[manager.IndexToNode(previous_index)]
                    curr_node = locs[manager.IndexToNode(index)]

                    if self.dist_mat:
                        cost += self.dist_mat[(prev_node[0:2], curr_node[0:2])] \
                                if prev_node != (-1, -1, -1, -1) and \
                                curr_node != (-1, -1, -1, -1) else 0.0
                    else:
                        cost += utils.travel_time(
                            prev_node, curr_node,
                            self.drones[vehicle_id]['speed'],
                            self.interest_map[curr_node]['task_time_seconds']
                            if curr_node[2] != -1 else 0.0) if prev_node != (
                                -1, -1, -1, -1) and curr_node != (-1, -1, -1, -1) else 0.0

                end = manager.IndexToNode(index)
                routes[vehicle_id]['path'] += [{
                    'location': {'latitude': locs[end][0], 'longitude': locs[end][1]},
                    'app_id': locs[end][2],
                    'request_time': locs[end][3],
                    'fulfill_time': int(cost)
                }]

                # verify that: route cost is within budget
                assert cost <= self.budget, \
                    "cost {}, ortools cost {}, budget {}, route {}".format(
                        cost, route_time, self.budget, routes[vehicle_id]['path']
                    )
                routes[vehicle_id]['vehicle_start'] = routes[vehicle_id]['path'][
                    0]['location']
                routes[vehicle_id]['vehicle_end'] = \
                    routes[vehicle_id]['path'][-1]['location'] \
                    if routes[vehicle_id]['path'][-1]['location']['latitude'] != -1 \
                    else routes[vehicle_id]['path'][-2]['location']

                # verify that path ends are right location
                if self.rth:
                    drone_end = (\
                            routes[vehicle_id]['vehicle_end']['latitude'], \
                            routes[vehicle_id]['vehicle_end']['longitude'])
                    depot = tuple(self.rth[vehicle_id][0:2])
                    assert drone_end == depot, \
                        "invalid end location {}, expected {}".format(drone_end, depot)
                else:
                    assert locs[end] == (-1, -1, -1, -1), \
                        "invalid end location {}, expected (-1, -1, -1, -1)".format(locs[end])

                routes[vehicle_id]['path'] = routes[vehicle_id]['path'][1:-1]
                routes[vehicle_id]['total_time'] = int(cost) 
                routes[vehicle_id]['total_interest'] = total_interest

            return routes

        def run_solver(first_solution_heuristic):
            cells = []
            for k, v in self.interest_map.items():
                cells += [k]

            for i in range(0, len(self.drones)):
                self.drones[i]['gps'] = (float(self.drones[i]['location']['latitude']),
                                         float(self.drones[i]['location']['longitude']), -1, -1)
                if self.drones[i]['gps'] not in cells:
                    cells += [self.drones[i]['gps']]

            if self.rth:
                for i in range(0, len(self.rth)):
                    self.rth[i] = (float(self.rth[i][0]), float(self.rth[i][1]),
                                   -1, -1)
                    if self.rth[i] not in cells:
                        cells += [self.rth[i]]

            if self.rth is None:
                cells = [(-1, -1, -1, -1)] + cells
            data = create_data_model(cells, self.initial_routes)
            manager = pywrapcp.RoutingIndexManager(len(data['distances']),
                                                   data['num_vehicles'],
                                                   data['start_locations'],
                                                   data['end_locations'])
            routing = pywrapcp.RoutingModel(manager)
            distance_callback = create_distance_callback(manager, data, cells)

            transit_callback_index = routing.RegisterTransitCallback(
                distance_callback)
            routing.SetArcCostEvaluatorOfAllVehicles(transit_callback_index)

            for node in range(1, len(cells)):
                cell = cells[node]
                if cell[2] != -1:
                    routing.AddDisjunction(
                        [manager.NodeToIndex(node)],
                        int(1e8 * self.interest_map[cell]['interest']))

            add_distance_dimension(routing, transit_callback_index,
                                   data['num_vehicles'])

            # set first solution heuristic
            search_parameters = pywrapcp.DefaultRoutingSearchParameters()
            search_parameters.first_solution_strategy = (
                first_solution_heuristic)

            # set local search
            if self.local_search:
                search_parameters.local_search_metaheuristic = (
                    routing_enums_pb2.LocalSearchMetaheuristic.
                    SIMULATED_ANNEALING)
                search_parameters.time_limit.seconds = 600
            else:
                search_parameters.time_limit.seconds = 10

            # solve using initial solution (if provided), else from scratch
            if self.initial_routes:
                initial_solution = routing.ReadAssignmentFromRoutes(
                    data['initial_routes'], True)
                assignment = routing.SolveFromAssignmentWithParameters(
                    initial_solution, search_parameters)
            else:
                assignment = routing.SolveWithParameters(search_parameters)

            if assignment:
                return parse_solution(data, manager, routing, assignment, cells)
            else:
                return None

    
        return run_solver(heuristic)
        

def convert_im(imj, uim = None):
    im = {}
    for t in imj:
        #assert(t['request_time'] == 0)
        if uim:
            key = (t['location']['latitude'], t['location']['longitude'], t['app_id'], t['request_time'])
            t['task_time_seconds'] += CAPACITY_TASK_BIAS * uim[key]['interest']
        im[(t['location']['latitude'], t['location']['longitude'], \
                t['app_id'], t['request_time'])] = t
    return im

def parse_dist_mat(mat):
    d = {}
    for x in mat:
        src_node = x['Dropoff']
        src = (src_node['latitude'], src_node['longitude'])
        dst_node = x['Pickup']
        dst = (dst_node['latitude'], dst_node['longitude'])
        d[(src, dst)] = x['TravelTime']
    return d

def run_solver(heuristic, im, v, b, cap, uim, dist, init, rth):
    solver = VRPSolver(
            im, 
            v, 
            b, 
            capacity=cap,
            unweighted_im=uim,
            dist_mat = dist,
            initial_schedule=init,
            rth = rth 
    )
    return solver.solve(heuristic)

if __name__ == '__main__':
    # read JSON-ized input from stdin
    inp = json.loads(sys.stdin.read())
    capacity = inp['capacity'] if inp['capacity'] > 0 else None
    unweighted_im = convert_im(inp['unweighted_interest_map']) if inp['unweighted_interest_map'] else im
    im = convert_im(inp['interest_map'], unweighted_im if capacity else None)
    initial_schedule = inp['initial_schedule'] if inp['initial_schedule']['routes'] else None
    if inp['rth']:
        rth = [(v['latitude'], v['longitude']) for v in inp['rth']]
    else:
        rth = None
    
    # load custom travel time matrix
    dist_mat = None
    if len(inp['travel_time_matrix_path']) > 0:
        with open(inp['travel_time_matrix_path']) as f:
            mat = json.load(f)
            dist_mat = parse_dist_mat(mat)
    
    im = collections.OrderedDict(sorted(im.items()))
    
    # run different first solution heuristics in parallel
    # choose most efficient solution
    pool = mp.Pool(processes = mp.cpu_count())
    inputs = [\
            (handler, im, inp['vehicles'], inp['budget'], capacity, unweighted_im, dist_mat, initial_schedule, rth)\
            for _, handler in FIRST_SOLUTION_HEURISTICS.items()]
    routes = pool.starmap(run_solver, inputs)
    
    best = (0, 0.0)
    idx = 0
    for r in routes:
        interests, _ = utils.get_schedule_stats(im, r)
        total = sum([i for app, i in interests.items()])
        best = (idx, total) if total >= best[1] else best
        idx += 1
    
    best_schedule = routes[best[0]]
    interests, _ = utils.get_schedule_stats(unweighted_im, best_schedule) if unweighted_im \
        else utils.get_schedule_stats(im, best_schedule)
    sched = {'routes': best_schedule, 'allocation': interests}
    
    print(json.dumps(sched))

