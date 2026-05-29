package io.killedkenny.crossfuzz;

import java.io.IOException;
import java.io.InputStream;
import java.lang.instrument.ClassFileTransformer;
import java.security.ProtectionDomain;
import java.util.ArrayList;
import java.util.HashSet;
import java.util.List;
import java.util.Set;
import org.objectweb.asm.ClassReader;
import org.objectweb.asm.ClassWriter;
import org.objectweb.asm.Opcodes;
import org.objectweb.asm.tree.AbstractInsnNode;
import org.objectweb.asm.tree.ClassNode;
import org.objectweb.asm.tree.InsnList;
import org.objectweb.asm.tree.JumpInsnNode;
import org.objectweb.asm.tree.LabelNode;
import org.objectweb.asm.tree.LdcInsnNode;
import org.objectweb.asm.tree.LookupSwitchInsnNode;
import org.objectweb.asm.tree.MethodInsnNode;
import org.objectweb.asm.tree.MethodNode;
import org.objectweb.asm.tree.TableSwitchInsnNode;
import org.objectweb.asm.tree.TryCatchBlockNode;

public class CoverageTransformer implements ClassFileTransformer {

    // The system (application) class loader is where -javaagent jars are placed.
    // Any loader that does not have this in its parent chain cannot find
    // CoverageRuntime and must not be instrumented.
    private static final ClassLoader SYS_LOADER = ClassLoader.getSystemClassLoader();

    @Override
    public byte[] transform(ClassLoader loader, String className,
            Class<?> classBeingRedefined, ProtectionDomain pd, byte[] buf) {
        if (className == null) return null;
        // Bootstrap-loaded classes (loader == null) and platform-module classes
        // (JDK 9+ platform class loader, e.g. jdk.localedata) are above the
        // system class loader in the hierarchy and cannot see CoverageRuntime —
        // the -javaagent jar is only on the system/app class loader's path.
        // Instrumenting them would cause NoClassDefFoundError at runtime.
        if (!canSeeRuntime(loader)) return null;
        // Never instrument the harness runtime or its ASM dependency.
        if (className.startsWith("io/killedkenny/crossfuzz/")) return null;
        if (className.startsWith("org/objectweb/asm/")) return null;
        try {
            ClassReader cr = new ClassReader(buf);
            ClassNode cn = new ClassNode();
            // SKIP_FRAMES: discard existing frames; COMPUTE_FRAMES rewrites them
            // from scratch so probe insertions never invalidate Uninitialized
            // type offsets or any other frame-tracked state.
            cr.accept(cn, ClassReader.SKIP_FRAMES);
            for (MethodNode mn : cn.methods) {
                instrumentMethod(className, mn);
            }
            // COMPUTE_FRAMES recomputes all stack map frames from scratch.
            // getCommonSuperClass uses getResourceAsStream rather than
            // Class.forName so it never calls defineClass — avoiding the
            // re-entrant loadClass → duplicate-definition LinkageError that
            // occurs when Class.forName is called for the class currently
            // being transformed.
            ClassWriter cw = new ClassWriter(ClassWriter.COMPUTE_FRAMES) {
                @Override
                protected String getCommonSuperClass(String type1, String type2) {
                    return commonSuperClass(type1, type2, loader);
                }
            };
            cn.accept(cw);
            return cw.toByteArray();
        } catch (Throwable t) {
            System.err.println("[crossfuzz] instrument failed: " + className + ": " + t);
            return null;
        }
    }

    // Returns true if loader is the system class loader or a descendant of it.
    // The -javaagent jar sits on the system class loader's classpath; any loader
    // that delegates to it (directly or transitively) can find CoverageRuntime.
    // The platform class loader and bootstrap class loader sit ABOVE the system
    // loader and do not delegate down, so they cannot find it.
    private static boolean canSeeRuntime(ClassLoader loader) {
        if (SYS_LOADER == null) return loader != null; // unusual env: skip bootstrap only
        for (ClassLoader cl = loader; cl != null; cl = cl.getParent()) {
            if (cl == SYS_LOADER) return true;
        }
        return false;
    }

    /**
     * Resolves the common superclass of two internal type names by reading
     * class bytes via getResourceAsStream rather than Class.forName.
     *
     * <p>Class.forName triggers ClassLoader.defineClass. When called inside
     * transform() for the class currently being defined, the same-thread
     * re-entrant loadClass call finds the class not yet defined, loads it
     * again, defines it, and then the outer transform's defineClass call
     * throws LinkageError: duplicate class definition.
     *
     * <p>getResourceAsStream reads bytes from the classpath without side
     * effects on the class loader state, so it is safe to call at any point
     * during transformation. Class.forName is used as a last resort only for
     * types that have no class file on the path (array types, dynamic proxies,
     * bootstrap module classes): those are always already loaded by the time
     * the agent runs, so defineClass is never triggered.
     */
    private static String commonSuperClass(String type1, String type2,
            ClassLoader loader) {
        if ("java/lang/Object".equals(type1) || "java/lang/Object".equals(type2)) {
            return "java/lang/Object";
        }
        List<String> chain1 = buildChain(type1, loader);
        if (chain1.contains(type2)) return type2;
        List<String> chain2 = buildChain(type2, loader);
        if (chain2.contains(type1)) return type1;
        Set<String> set1 = new HashSet<>(chain1);
        for (String ancestor : chain2) {
            if (set1.contains(ancestor)) return ancestor;
        }
        return "java/lang/Object";
    }

    private static List<String> buildChain(String type, ClassLoader loader) {
        List<String> chain = new ArrayList<>();
        Set<String> seen = new HashSet<>();
        String cur = type;
        while (cur != null && !cur.equals("java/lang/Object") && seen.add(cur)) {
            chain.add(cur);
            cur = superOf(cur, loader);
        }
        chain.add("java/lang/Object");
        return chain;
    }

    private static String superOf(String internalName, ClassLoader loader) {
        // Try both the target loader and the system loader (for bootstrap types
        // that surface as resources in the JDK's jimage/modules on JDK 9+).
        ClassLoader sys = ClassLoader.getSystemClassLoader();
        for (ClassLoader cl : new ClassLoader[]{loader, sys}) {
            if (cl == null) continue;
            try (InputStream is = cl.getResourceAsStream(internalName + ".class")) {
                if (is != null) {
                    return new ClassReader(is).getSuperName();
                }
            } catch (IOException ignored) {}
        }
        // Class not findable as a resource: array type, dynamic proxy, or a
        // bootstrap class whose module doesn't export resources. These are
        // always already loaded, so Class.forName won't call defineClass.
        try {
            ClassLoader cl = loader != null ? loader : sys;
            Class<?> c = Class.forName(internalName.replace('/', '.'), false, cl);
            Class<?> sup = c.getSuperclass();
            return sup != null ? sup.getName().replace('.', '/') : null;
        } catch (Exception ignored) {
            return null;
        }
    }

    private void instrumentMethod(String cls, MethodNode mn) {
        // Abstract and native methods have no body. Inserting any instruction
        // would give them a Code attribute, which JVMS §4.7.3 forbids — the JVM
        // then rejects the class with ClassFormatError at defineClass time (after
        // transform() has already returned, so the catch in transform() cannot
        // recover from it). Skip these methods entirely.
        if ((mn.access & (Opcodes.ACC_ABSTRACT | Opcodes.ACC_NATIVE)) != 0) {
            return;
        }

        // Collect branch target labels (basic block entries)
        Set<LabelNode> targets = new HashSet<>();
        for (AbstractInsnNode n : mn.instructions.toArray()) {
            if (n instanceof JumpInsnNode) {
                targets.add(((JumpInsnNode) n).label);
            } else if (n instanceof TableSwitchInsnNode) {
                TableSwitchInsnNode ts = (TableSwitchInsnNode) n;
                targets.add(ts.dflt);
                ts.labels.forEach(targets::add);
            } else if (n instanceof LookupSwitchInsnNode) {
                LookupSwitchInsnNode ls = (LookupSwitchInsnNode) n;
                targets.add(ls.dflt);
                ls.labels.forEach(targets::add);
            }
        }
        for (TryCatchBlockNode tcb : mn.tryCatchBlocks) {
            targets.add(tcb.handler);
        }

        // Inject at method entry (before first instruction)
        mn.instructions.insert(makeHit(cls, mn.name, 0));

        // Inject after each branch-target label.
        // insert(n, probe) puts the probe BEFORE the LabelNode, but the label's
        // bytecode offset equals the first real instruction AFTER it — so a taken
        // branch skips the probe entirely. Instead we insert the probe before the
        // node that currently follows the label in the live list. That node becomes
        // the second item after the label; the probe becomes the first, so the
        // label's bytecode offset now points to the probe. Taken branches land on
        // the probe; fall-through paths also run it.
        int blockId = 1;
        for (AbstractInsnNode n : mn.instructions.toArray()) {
            if (n instanceof LabelNode && targets.contains(n)) {
                InsnList probe = makeHit(cls, mn.name, blockId++);
                AbstractInsnNode after = n.getNext();
                if (after != null) {
                    mn.instructions.insert(after, probe);
                } else {
                    mn.instructions.add(probe);
                }
            }
        }
    }

    private InsnList makeHit(String cls, String method, int blockId) {
        int idx = (cls + "_" + method + "_" + blockId).hashCode() & 0xFFFF;
        InsnList l = new InsnList();
        l.add(new LdcInsnNode(idx));
        l.add(new MethodInsnNode(Opcodes.INVOKESTATIC,
            "io/killedkenny/crossfuzz/CoverageRuntime", "hit", "(I)V", false));
        return l;
    }
}
