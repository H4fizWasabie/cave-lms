import bpy, math, sys
from mathutils import Vector

out = sys.argv[sys.argv.index('--') + 1]
W, H, SAMPLES = 1200, 1600, int(sys.argv[sys.argv.index('--') + 2])

bpy.ops.wm.read_factory_settings(use_empty=True)
sc = bpy.context.scene

# ---------- arch profile (the gold "A" from the logo: a full pointed arch with a cave doorway) ----------
def edge(y_top, half, k, n=48):
    pts = []
    for i in range(n + 1):
        y = y_top * i / n
        pts.append((-half * (1 - y / y_top) ** k, y))
    return pts

outer_left = edge(2.6, 1.0, 0.72)
inner_left = edge(1.45, 0.40, 0.55)
def closed(left):
    right = [(-x, y) for (x, y) in reversed(left[:-1])]
    return left + right

def make_curve(name, loops, extrude, bevel):
    cu = bpy.data.curves.new(name, 'CURVE')
    cu.dimensions = '2D'; cu.fill_mode = 'BOTH'
    cu.extrude = extrude; cu.bevel_depth = bevel; cu.bevel_resolution = 4
    for loop in loops:
        sp = cu.splines.new('POLY'); sp.points.add(len(loop) - 1)
        for p, (x, y) in zip(sp.points, loop): p.co = (x, y, 0, 1)
        sp.use_cyclic_u = True
    ob = bpy.data.objects.new(name, cu); sc.collection.objects.link(ob)
    ob.rotation_euler = (math.radians(90), 0, 0)
    return ob

arch = make_curve('Arch', [closed(outer_left), closed(inner_left)], 0.3, 0.035)
glow = make_curve('Glow', [closed(inner_left)], 0.0, 0.0)
glow.location.y = 0.42

# ---------- materials ----------
def principled(name, color, metal, rough, emit=None, strength=0):
    m = bpy.data.materials.new(name); m.use_nodes = True
    b = m.node_tree.nodes['Principled BSDF']
    b.inputs['Base Color'].default_value = (*color, 1)
    b.inputs['Metallic'].default_value = metal
    b.inputs['Roughness'].default_value = rough
    if emit:
        b.inputs['Emission Color'].default_value = (*emit, 1)
        b.inputs['Emission Strength'].default_value = strength
    return m

gold = principled('Gold', (1.0, 0.64, 0.23), 1.0, 0.24)
arch.data.materials.append(gold)
glow.data.materials.append(principled('Glow', (0, 0, 0), 0, 1, emit=(1.0, 0.56, 0.2), strength=6))

bpy.ops.mesh.primitive_plane_add(size=40, location=(0, 0, 0))
floor = bpy.context.object
floor.data.materials.append(principled('Floor', (0.018, 0.019, 0.021), 0.0, 0.32))

bpy.ops.mesh.primitive_plane_add(size=40, location=(0, 6, 10), rotation=(math.radians(90), 0, 0))
bpy.context.object.data.materials.append(principled('Wall', (0.012, 0.012, 0.014), 0.0, 0.9))

# ---------- reflectors (metal needs something bright to mirror) ----------
def softbox(loc, rot, sx, sy, strength, color=(1.0, 0.93, 0.82)):
    bpy.ops.mesh.primitive_plane_add(size=1, location=loc, rotation=rot)
    ob = bpy.context.object; ob.scale = (sx, sy, 1)
    m = principled('SB', (0, 0, 0), 0, 1, emit=color, strength=strength); ob.data.materials.append(m)
    ob.visible_camera = False
softbox((-1.5, -9, 6.5), (math.radians(70), 0, math.radians(-10)), 5, 2.2, 1.1)
softbox((-5, -3, 2), (math.radians(90), 0, math.radians(-60)), 1.2, 6, 6)
softbox((5, -3, 2), (math.radians(90), 0, math.radians(60)), 1.2, 6, 4, (1.0, 0.8, 0.55))

# ---------- lights ----------
def area(loc, rot, size, power, color):
    bpy.ops.object.light_add(type='AREA', location=loc, rotation=rot)
    l = bpy.context.object.data; l.size = size; l.energy = power; l.color = color
area((-3.2, -2.4, 4.2), (math.radians(55), 0, math.radians(-50)), 2.5, 520, (1.0, 0.9, 0.78))
area((3.4, 1.8, 2.6), (math.radians(70), 0, math.radians(120)), 1.5, 420, (1.0, 0.82, 0.55))
bpy.ops.object.light_add(type='POINT', location=(0, 0.6, 0.7))
bpy.context.object.data.energy = 140; bpy.context.object.data.color = (1.0, 0.75, 0.42)
bpy.context.object.data.shadow_soft_size = 0.3

world = bpy.data.worlds.new('W'); sc.world = world; world.use_nodes = True
world.node_tree.nodes['Background'].inputs['Color'].default_value = (0.006, 0.006, 0.007, 1)

# ---------- camera ----------
bpy.ops.object.camera_add(location=(1.9, -7.0, 1.0))
cam = bpy.context.object; sc.camera = cam
cam.data.lens = 58
target = Vector((0, 0, 1.55))
cam.rotation_euler = (target - cam.location).to_track_quat('-Z', 'Y').to_euler()
cam.data.dof.use_dof = True; cam.data.dof.focus_distance = (target - cam.location).length; cam.data.dof.aperture_fstop = 2.8

# ---------- render ----------
sc.render.engine = 'CYCLES'
prefs = bpy.context.preferences.addons['cycles'].preferences
for kind in ('OPTIX', 'CUDA'):
    try:
        prefs.compute_device_type = kind; prefs.get_devices()
        if any(d.type == kind for d in prefs.devices):
            for d in prefs.devices: d.use = d.type in (kind, 'CPU')
            sc.cycles.device = 'GPU'; print('device', kind); break
    except Exception as e:
        print('no', kind, e)
sc.cycles.samples = SAMPLES; sc.cycles.use_denoising = True
sc.render.resolution_x, sc.render.resolution_y = W, H
sc.view_settings.view_transform = 'AgX'; sc.view_settings.look = 'AgX - Punchy'
sc.render.image_settings.file_format = 'JPEG'; sc.render.image_settings.quality = 86
sc.render.filepath = out
bpy.ops.render.render(write_still=True)
print('wrote', out)
